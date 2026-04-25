package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
)

type captureStreamingTransport struct {
	body []byte
	path string
}

func (t *captureStreamingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		t.body = body
	}
	t.path = req.URL.Path

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(responsesStreamBody)),
		Request:    req,
	}, nil
}

const responsesStreamBody = "" +
	"data: {\"type\":\"response.reasoning_summary_text.delta\",\"item_id\":\"rs_1\",\"output_index\":0,\"summary_index\":0,\"delta\":\"thinking...\"}\n\n" +
	"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":1,\"content_index\":0,\"delta\":\"ok\"}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":2,\"output_tokens_details\":{\"reasoning_tokens\":1}}}}\n\n" +
	"data: [DONE]\n\n"

func newTestClient(t *captureStreamingTransport) *Client {
	client := sdk.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("https://example.com/v1"),
		option.WithHTTPClient(&http.Client{Transport: t}),
	)
	return NewClient(client, "openai:test")
}

func drain(ch <-chan llm.StreamChunk) []llm.StreamChunk {
	var chunks []llm.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func TestOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name  string
		model string
		level llm.ThinkingLevel
		want  string
	}{
		{name: "off on gpt-5.1 defaults to high", model: "gpt-5.1", level: llm.ThinkingOff, want: "high"},
		{name: "off on gpt-5 defaults to high", model: "gpt-5", level: llm.ThinkingOff, want: "high"},
		{name: "off on gpt-5.4 maps to none", model: "gpt-5.4", level: llm.ThinkingOff, want: "none"},
		{name: "off on gpt-5.5 maps to none", model: "gpt-5.5-2026-04-23", level: llm.ThinkingOff, want: "none"},
		{name: "off on o-series defaults to high", model: "o4-mini", level: llm.ThinkingOff, want: "high"},
		{name: "normal on o-series defaults to high", model: "o4-mini", level: llm.ThinkingNormal, want: "high"},
		{name: "high on o-series defaults to high", model: "o4-mini", level: llm.ThinkingHigh, want: "high"},
		{name: "ultra on gpt-5 maps to high", model: "gpt-5", level: llm.ThinkingUltra, want: "high"},
		{name: "ultra on gpt-5.4 maps to xhigh", model: "gpt-5.4", level: llm.ThinkingUltra, want: "xhigh"},
		{name: "off on codex defaults to high", model: "gpt-5.1-codex", level: llm.ThinkingOff, want: "high"},
		{name: "ultra on codex defaults to high", model: "gpt-5.1-codex-max", level: llm.ThinkingUltra, want: "high"},
		{name: "gpt-5-pro collapses to high", model: "gpt-5-pro", level: llm.ThinkingOff, want: "high"},
		{name: "non reasoning models stay unset", model: "gpt-4.1", level: llm.ThinkingHigh, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, ok := openAIReasoningProfile(tt.model)
			if !ok && tt.want != "" {
				t.Fatalf("openAIReasoningProfile(%q) not found", tt.model)
			}
			if got := string(openaiReasoningEffort(profile, tt.level)); got != tt.want {
				t.Fatalf("openaiReasoningEffort(%q, %v) = %q, want %q", tt.model, tt.level, got, tt.want)
			}
		})
	}
}

func TestStreamUsesResponsesAPIForGpt54(t *testing.T) {
	transport := &captureStreamingTransport{}
	client := newTestClient(transport)

	drain(client.Stream(context.Background(), llm.CompletionOptions{
		Model:         "gpt-5.4",
		Messages:      []core.Message{{Role: core.RoleUser, Content: "hi"}},
		ThinkingLevel: llm.ThinkingOff,
	}))

	if transport.path != "/v1/responses" {
		t.Fatalf("expected responses path, got %q", transport.path)
	}

	var payload map[string]any
	if err := json.Unmarshal(transport.body, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}

	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object in payload")
	}
	if got, _ := reasoning["effort"].(string); got != "none" {
		t.Fatalf("expected reasoning.effort=none, got %#v", reasoning["effort"])
	}
}

func TestStreamResponsesIncludesReasoningSummaryAndEmitsThinking(t *testing.T) {
	transport := &captureStreamingTransport{}
	client := newTestClient(transport)

	chunks := drain(client.Stream(context.Background(), llm.CompletionOptions{
		Model:         "gpt-5.4",
		Messages:      []core.Message{{Role: core.RoleUser, Content: "hi"}},
		ThinkingLevel: llm.ThinkingUltra,
	}))

	if transport.path != "/v1/responses" {
		t.Fatalf("expected responses path, got %q", transport.path)
	}

	var payload map[string]any
	if err := json.Unmarshal(transport.body, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}

	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object in payload")
	}
	if got, _ := reasoning["effort"].(string); got != "xhigh" {
		t.Fatalf("expected reasoning.effort=xhigh, got %#v", reasoning["effort"])
	}
	if got, _ := reasoning["summary"].(string); got != "auto" {
		t.Fatalf("expected reasoning.summary=auto, got %#v", reasoning["summary"])
	}

	foundThinking := false
	for _, chunk := range chunks {
		if chunk.Type == llm.ChunkTypeThinking && chunk.Text == "thinking..." {
			foundThinking = true
			break
		}
	}
	if !foundThinking {
		t.Fatal("expected reasoning summary delta to emit a thinking chunk")
	}
}

func TestStreamResponsesIncludesImageInputs(t *testing.T) {
	transport := &captureStreamingTransport{}
	client := newTestClient(transport)

	drain(client.Stream(context.Background(), llm.CompletionOptions{
		Model: "gpt-5.4",
		Messages: []core.Message{{
			Role:    core.RoleUser,
			Content: "describe this",
			Images: []core.Image{{
				MediaType: "image/png",
				Data:      "ZmFrZQ==",
			}},
		}},
	}))

	var payload map[string]any
	if err := json.Unmarshal(transport.body, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}

	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		t.Fatalf("expected input items in payload")
	}
	message, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first input item to be a message")
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("expected text+image content array, got %#v", message["content"])
	}

	var imagePart map[string]any
	for _, part := range content {
		item, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if got, _ := item["type"].(string); got == "input_image" {
			imagePart = item
			break
		}
	}
	if imagePart == nil {
		t.Fatalf("expected at least one input_image content part, got %#v", content)
	}
	if got, _ := imagePart["image_url"].(string); got != "data:image/png;base64,ZmFrZQ==" {
		t.Fatalf("expected data URL image, got %#v", imagePart["image_url"])
	}
}

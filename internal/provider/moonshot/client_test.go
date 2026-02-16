package moonshot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

type captureTransport struct {
	body []byte
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		t.body = b
	}

	streamBody := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(io.Reader(strings.NewReader(streamBody))),
	}
	return resp, nil
}

func TestMoonshotAssistantMessagesIncludeReasoningContent(t *testing.T) {
	transport := &captureTransport{}
	client := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("https://example.com/v1"),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	)

	c := NewClient(client, "moonshot:test")

	messages := []message.Message{
		{Role: message.RoleUser, Content: "hi"},
		{Role: message.RoleAssistant, ToolCalls: []message.ToolCall{{ID: "tc1", Name: "WebSearch", Input: "{}"}}},
		{Role: message.RoleUser, ToolResult: &message.ToolResult{ToolCallID: "tc1", Content: "ok"}},
		{Role: message.RoleAssistant, Content: "done"},
	}

	ch := c.Stream(context.Background(), provider.CompletionOptions{
		Model:        "kimi-k2.5",
		Messages:     messages,
		SystemPrompt: "sys",
	})
	for range ch {
	}

	if len(transport.body) == 0 {
		t.Fatal("no request body captured")
	}

	var payload map[string]any
	if err := json.Unmarshal(transport.body, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}

	rawMsgs, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages not found in payload")
	}

	for i, raw := range rawMsgs {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		if _, ok := msg["reasoning_content"]; !ok {
			t.Fatalf("assistant message missing reasoning_content at index %d", i)
		}
	}
}

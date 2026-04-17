// Package moonshot implements the Provider interface using the Moonshot AI platform.
// Moonshot's API is OpenAI-compatible, so we reuse the openai-go SDK with a custom base URL.
package moonshot

import (
	"context"
	"encoding/json"
	"cmp"
	"slices"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/llm/openaicompat"
	streamutil "github.com/yanmxa/gencode/internal/llm/stream"
)

// Client implements the Provider interface for Moonshot AI using the OpenAI SDK.
type Client struct {
	client openai.Client
	name   string
}

// NewClient creates a new Moonshot client with the given OpenAI SDK client.
func NewClient(client openai.Client, name string) *Client {
	return &Client{
		client: client,
		name:   name,
	}
}

// Name returns the provider name.
func (c *Client) Name() string {
	return c.name
}

// convertAssistant converts an assistant message for Moonshot.
// Moonshot requires reasoning_content on all assistant messages when thinking
// is enabled — we always include the field (empty string if no thinking content).
func convertAssistant(msg core.Message) openai.ChatCompletionMessageParamUnion {
	return openaicompat.AssistantMessageWithReasoning(msg, msg.Thinking)
}

// Stream sends a completion request and returns a channel of streaming chunks.
func (c *Client) Stream(ctx context.Context, opts llm.CompletionOptions) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)

	go func() {
		defer close(ch)

		messages := openaicompat.ConvertMessages(opts.Messages, opts.SystemPrompt, convertAssistant)

		params := openai.ChatCompletionNewParams{
			Model:    opts.Model,
			Messages: messages,
		}

		// Enable thinking mode only when explicitly requested.
		// Unconditionally enabling thinking on non-thinking models (e.g. moonshot-v1-auto)
		// causes API errors. The caller must set opts.ThinkingLevel for Kimi thinking models.
		if opts.ThinkingLevel > llm.ThinkingOff {
			params.SetExtraFields(map[string]any{
				"thinking": map[string]any{"type": "enabled"},
			})
		}

		if opts.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(int64(opts.MaxTokens))
		}
		if opts.Temperature > 0 {
			params.Temperature = openai.Float(opts.Temperature)
		}
		if len(opts.Tools) > 0 {
			params.Tools = openaicompat.ConvertTools(opts.Tools)
		}

		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		stream := c.client.Chat.Completions.NewStreaming(ctx, params)
		state := streamutil.NewState(c.name)
		toolCalls := make(map[int]*core.ToolCall)

		for stream.Next() {
			chunk := stream.Current()
			state.Count()

			for _, choice := range chunk.Choices {
				// Extract reasoning_content for Kimi thinking models
				if content := openaicompat.ExtractReasoningContent(choice.Delta.RawJSON()); content != "" {
					state.EmitThinking(ch, content)
				}

				if choice.Delta.Content != "" {
					state.EmitText(ch, choice.Delta.Content)
				}

				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)
					if _, exists := toolCalls[idx]; !exists {
						toolCalls[idx] = &core.ToolCall{ID: tc.ID, Name: tc.Function.Name}
						state.EmitToolStart(ch, tc.ID, tc.Function.Name)
					}
					if tc.Function.Arguments != "" {
						toolCalls[idx].Input += tc.Function.Arguments
						state.EmitToolInput(ch, toolCalls[idx].ID, tc.Function.Arguments)
					}
				}

				if choice.FinishReason != "" {
					state.Response.StopReason = openaicompat.MapFinishReason(choice.FinishReason)
				}
			}

			state.UpdateUsage(int(chunk.Usage.PromptTokens), int(chunk.Usage.CompletionTokens))
		}

		if err := stream.Err(); err != nil {
			state.Fail(ch, err)
			return
		}

		state.AddToolCallsSorted(toolCalls)
		state.EnsureToolUseStopReason()
		state.Finish(ctx, ch)
	}()

	return ch
}

// staticModels is the fallback list when the models API is unavailable.
var staticModels = []llm.ModelInfo{
	{ID: "moonshot-v1-auto", Name: "moonshot-v1-auto", DisplayName: "Moonshot V1 Auto"},
	{ID: "moonshot-v1-128k", Name: "moonshot-v1-128k", DisplayName: "Moonshot V1 128K"},
	{ID: "kimi-k2-0711-preview", Name: "kimi-k2-0711-preview", DisplayName: "Kimi K2 0711 Preview"},
	{ID: "kimi-k2-0905-preview", Name: "kimi-k2-0905-preview", DisplayName: "Kimi K2 0905 Preview"},
}

// ListModels returns the available models for Moonshot AI using the API.
func (c *Client) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	page, err := c.client.Models.List(ctx)
	if err != nil {
		return staticModels, nil // graceful fallback to static list
	}

	models := make([]llm.ModelInfo, 0, len(page.Data))
	for _, m := range page.Data {
		id := m.ID
		info := llm.ModelInfo{ID: id, Name: id, DisplayName: id}
		// Extract context_length from raw JSON (Moonshot extension field)
		if raw := m.RawJSON(); raw != "" {
			var extra struct {
				ContextLength int `json:"context_length"`
			}
			if err := json.Unmarshal([]byte(raw), &extra); err == nil && extra.ContextLength > 0 {
				info.InputTokenLimit = extra.ContextLength
			}
		}
		models = append(models, info)
	}

	if len(models) == 0 {
		return staticModels, nil
	}

	slices.SortFunc(models, func(a, b llm.ModelInfo) int { return cmp.Compare(a.ID, b.ID) })
	return models, nil
}

// Ensure Client implements Provider
var _ llm.Provider = (*Client)(nil)

// Package alibaba implements the Provider interface using the Alibaba Cloud DashScope platform.
// DashScope's API is OpenAI-compatible, so we reuse the openai-go SDK with a custom base URL.
package alibaba

import (
	"context"
	"encoding/json"
	"cmp"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/openaicompat"
	streamutil "github.com/yanmxa/gencode/internal/provider/stream"
)

// Client implements the Provider interface for Qwen using the OpenAI SDK.
type Client struct {
	client openai.Client
	name   string
}

// NewClient creates a new Qwen client with the given OpenAI SDK client.
func NewClient(client openai.Client, name string) *Client {
	return &Client{client: client, name: name}
}

// Name returns the provider name.
func (c *Client) Name() string { return c.name }

// isThinkingModel returns true if the model supports thinking/reasoning mode.
func isThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "qwq") || strings.Contains(lower, "thinking")
}

// makeAssistantConverter returns a provider-specific assistant message converter.
// For thinking models, reasoning_content is injected only when present.
func makeAssistantConverter(thinking bool) func(core.Message) openai.ChatCompletionMessageParamUnion {
	if !thinking {
		return openaicompat.DefaultAssistantMessage
	}
	return func(msg core.Message) openai.ChatCompletionMessageParamUnion {
		return openaicompat.AssistantMessageWithReasoning(msg, msg.Thinking)
	}
}

// Stream sends a completion request and returns a channel of streaming chunks.
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan core.StreamChunk {
	ch := make(chan core.StreamChunk)

	go func() {
		defer close(ch)

		thinking := isThinkingModel(opts.Model)

		messages := openaicompat.ConvertMessages(opts.Messages, opts.SystemPrompt, makeAssistantConverter(thinking))

		params := openai.ChatCompletionNewParams{
			Model:    opts.Model,
			Messages: messages,
		}

		if thinking {
			extraFields := map[string]any{"enable_thinking": true}
			if budget := opts.ThinkingLevel.BudgetTokens(); budget > 0 {
				extraFields["thinking_budget"] = budget
			}
			params.SetExtraFields(extraFields)
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
				// Extract reasoning_content for thinking models
				if thinking {
					if content := openaicompat.ExtractReasoningContent(choice.Delta.RawJSON()); content != "" {
						state.EmitThinking(ch, content)
					}
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

// ListModels returns the available models for Qwen by querying the DashScope API.
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	page, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, err
	}

	models := make([]provider.ModelInfo, 0, len(page.Data))
	for _, m := range page.Data {
		models = append(models, provider.ModelInfo{ID: m.ID, Name: m.ID, DisplayName: m.ID})
	}

	slices.SortFunc(models, func(a, b provider.ModelInfo) int { return cmp.Compare(a.ID, b.ID) })
	return models, nil
}

// FetchModelLimits queries the DashScope model detail API to get token limits.
// DashScope returns extra_info.default_envs with max_input_tokens and max_output_tokens.
func (c *Client) FetchModelLimits(ctx context.Context, modelID string) (inputLimit, outputLimit int, err error) {
	model, err := c.client.Models.Get(ctx, modelID)
	if err != nil {
		return 0, 0, err
	}

	raw := model.RawJSON()
	if raw == "" {
		return 0, 0, nil
	}

	var detail struct {
		ExtraInfo struct {
			DefaultEnvs struct {
				MaxInputTokens  int `json:"max_input_tokens"`
				MaxOutputTokens int `json:"max_output_tokens"`
			} `json:"default_envs"`
		} `json:"extra_info"`
	}
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		return 0, 0, nil
	}

	return detail.ExtraInfo.DefaultEnvs.MaxInputTokens, detail.ExtraInfo.DefaultEnvs.MaxOutputTokens, nil
}

// Ensure Client implements Provider and ModelLimitsFetcher
var _ provider.Provider = (*Client)(nil)
var _ provider.ModelLimitsFetcher = (*Client)(nil)

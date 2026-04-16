package openai

import (
	"context"
	"fmt"
	"cmp"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/openaicompat"
	streamutil "github.com/yanmxa/gencode/internal/provider/stream"
)

// Client implements the Provider interface using the OpenAI SDK
type Client struct {
	client openai.Client
	name   string
}

// NewClient creates a new OpenAI client with the given SDK client
func NewClient(client openai.Client, name string) *Client {
	return &Client{
		client: client,
		name:   name,
	}
}

// Name returns the provider name
func (c *Client) Name() string {
	return c.name
}

// isResponsesModel returns true if the model uses the Responses API instead of Chat Completions.
func isResponsesModel(model string) bool {
	return strings.Contains(model, "codex")
}

// Stream sends a completion request and returns a channel of streaming chunks.
// It routes to the Responses API for codex models and Chat Completions for all others.
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan core.StreamChunk {
	if isResponsesModel(opts.Model) {
		return c.streamResponses(ctx, opts)
	}
	return c.streamChatCompletions(ctx, opts)
}

// streamResponses implements streaming via the Responses API for codex models.
func (c *Client) streamResponses(ctx context.Context, opts provider.CompletionOptions) <-chan core.StreamChunk {
	ch := make(chan core.StreamChunk)

	go func() {
		defer close(ch)

		// Convert messages to Responses API input items
		var inputItems responses.ResponseInputParam = make([]responses.ResponseInputItemUnionParam, 0, len(opts.Messages)+1)

		for _, msg := range opts.Messages {
			switch msg.Role {
			case core.RoleUser:
				if msg.ToolResult != nil {
					inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							CallID: msg.ToolResult.ToolCallID,
							Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
								OfString: openai.Opt(msg.ToolResult.Content),
							},
						},
					})
				} else {
					inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
						OfMessage: &responses.EasyInputMessageParam{
							Role: responses.EasyInputMessageRoleUser,
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: openai.Opt(msg.Content),
							},
						},
					})
				}
			case core.RoleAssistant:
				if len(msg.ToolCalls) > 0 {
					// Add text content as a message if present
					if msg.Content != "" {
						inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
							OfMessage: &responses.EasyInputMessageParam{
								Role: responses.EasyInputMessageRoleAssistant,
								Content: responses.EasyInputMessageContentUnionParam{
									OfString: openai.Opt(msg.Content),
								},
							},
						})
					}
					// Add each tool call as a separate function_call input item
					for _, tc := range msg.ToolCalls {
						inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
							OfFunctionCall: &responses.ResponseFunctionToolCallParam{
								CallID:    tc.ID,
								Name:      tc.Name,
								Arguments: tc.Input,
							},
						})
					}
				} else {
					inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
						OfMessage: &responses.EasyInputMessageParam{
							Role: responses.EasyInputMessageRoleAssistant,
							Content: responses.EasyInputMessageContentUnionParam{
								OfString: openai.Opt(msg.Content),
							},
						},
					})
				}
			default: // system messages
				inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleSystem,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.Opt(msg.Content),
						},
					},
				})
			}
		}

		// Build request params
		params := responses.ResponseNewParams{
			Model: opts.Model,
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: inputItems,
			},
		}

		if opts.SystemPrompt != "" {
			params.Instructions = openai.Opt(opts.SystemPrompt)
		}

		if opts.MaxTokens > 0 {
			params.MaxOutputTokens = openai.Opt(int64(opts.MaxTokens))
		}

		if opts.Temperature > 0 {
			params.Temperature = openai.Opt(opts.Temperature)
		}

		// Configure reasoning effort for o-series models
		if effort := openaiReasoningEffort(opts.ThinkingLevel); effort != "" {
			params.Reasoning = shared.ReasoningParam{
				Effort: effort,
			}
		}

		// Add tools if provided
		if len(opts.Tools) > 0 {
			tools := make([]responses.ToolUnionParam, len(opts.Tools))
			for i, t := range opts.Tools {
				var funcParams map[string]any
				if props, ok := t.Parameters.(map[string]any); ok {
					funcParams = props
				}
				tools[i] = responses.ToolUnionParam{
					OfFunction: &responses.FunctionToolParam{
						Name:        t.Name,
						Description: openai.Opt(t.Description),
						Parameters:  funcParams,
					},
				}
			}
			params.Tools = tools
		}

		// Log request
		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Responses.NewStreaming(ctx, params)

		state := streamutil.NewState(c.name)

		// Track tool calls by item ID
		toolCalls := make(map[string]*core.ToolCall)
		hasToolCalls := false

		// Read stream events
		for stream.Next() {
			event := stream.Current()
			state.Count()

			switch event.Type {
			case "response.output_text.delta":
				delta := event.AsResponseOutputTextDelta()
				state.EmitText(ch, delta.Delta)

			case "response.output_item.added":
				itemEvent := event.AsResponseOutputItemAdded()
				if itemEvent.Item.Type == "function_call" {
					funcCall := itemEvent.Item.AsFunctionCall()
					hasToolCalls = true
					toolCalls[funcCall.ID] = &core.ToolCall{
						ID:   funcCall.CallID,
						Name: funcCall.Name,
					}
					state.EmitToolStart(ch, funcCall.CallID, funcCall.Name)
				}

			case "response.function_call_arguments.delta":
				delta := event.AsResponseFunctionCallArgumentsDelta()
				if tc, ok := toolCalls[delta.ItemID]; ok {
					tc.Input += delta.Delta
					state.EmitToolInput(ch, tc.ID, delta.Delta)
				}

			case "response.completed":
				completed := event.AsResponseCompleted()
				resp := completed.Response

				// Map usage
				state.UpdateUsage(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens))

				// Determine stop reason
				switch resp.Status {
				case responses.ResponseStatusCompleted:
					if hasToolCalls {
						state.Response.StopReason = "tool_use"
					} else {
						state.Response.StopReason = "end_turn"
					}
				case responses.ResponseStatusIncomplete:
					state.Response.StopReason = "max_tokens"
				case responses.ResponseStatusFailed:
					errMsg := "response failed"
					if resp.Error.Message != "" {
						errMsg = resp.Error.Message
					}
					state.Fail(ch, fmt.Errorf("responses API failed: %s", errMsg))
					return
				default:
					state.Response.StopReason = string(resp.Status)
				}

			case "error":
				errEvent := event.AsError()
				state.Fail(ch, fmt.Errorf("responses API error: %s", errEvent.Message))
				return
			}
		}

		if err := stream.Err(); err != nil {
			state.Fail(ch, err)
			return
		}

		state.AddToolCallsByKey(toolCalls)
		state.EnsureToolUseStopReason()
		state.Finish(ctx, ch)
	}()

	return ch
}

// streamChatCompletions implements streaming via the Chat Completions API.
func (c *Client) streamChatCompletions(ctx context.Context, opts provider.CompletionOptions) <-chan core.StreamChunk {
	ch := make(chan core.StreamChunk)

	go func() {
		defer close(ch)

		messages := openaicompat.ConvertMessages(opts.Messages, opts.SystemPrompt, openaicompat.DefaultAssistantMessage)

		params := openai.ChatCompletionNewParams{
			Model:    opts.Model,
			Messages: messages,
		}

		if opts.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(int64(opts.MaxTokens))
		}

		if opts.Temperature > 0 {
			params.Temperature = openai.Float(opts.Temperature)
		}

		// Configure reasoning effort for o-series models
		if effort := openaiReasoningEffort(opts.ThinkingLevel); effort != "" {
			params.ReasoningEffort = effort
		}

		// Add tools if provided
		if len(opts.Tools) > 0 {
			params.Tools = openaicompat.ConvertTools(opts.Tools)
		}

		// Log request
		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Chat.Completions.NewStreaming(ctx, params)

		state := streamutil.NewState(c.name)

		// Track tool calls
		toolCalls := make(map[int]*core.ToolCall)

		// Read stream events
		for stream.Next() {
			chunk := stream.Current()
			state.Count()

			for _, choice := range chunk.Choices {
				// Handle text delta
				if choice.Delta.Content != "" {
					state.EmitText(ch, choice.Delta.Content)
				}

				// Handle tool calls
				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)

					// Initialize new tool call
					if _, exists := toolCalls[idx]; !exists {
						toolCalls[idx] = &core.ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
						}
						state.EmitToolStart(ch, tc.ID, tc.Function.Name)
					}

					// Accumulate arguments
					if tc.Function.Arguments != "" {
						toolCalls[idx].Input += tc.Function.Arguments
						state.EmitToolInput(ch, toolCalls[idx].ID, tc.Function.Arguments)
					}
				}

				// Handle finish reason
				if choice.FinishReason != "" {
					state.Response.StopReason = openaicompat.MapFinishReason(choice.FinishReason)
				}
			}

			// Handle usage
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

// ListModels returns the available models for OpenAI using the API
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	// Use OpenAI API to dynamically fetch models
	page, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, err
	}

	models := make([]provider.ModelInfo, 0, len(page.Data))

	for _, m := range page.Data {
		id := m.ID
		// Skip models that don't support chat completions or responses API
		if strings.HasPrefix(id, "dall-e") ||
			strings.HasPrefix(id, "tts-") ||
			strings.HasPrefix(id, "whisper-") ||
			strings.HasPrefix(id, "text-embedding") ||
			strings.HasPrefix(id, "omni-moderation") ||
			strings.HasPrefix(id, "davinci") ||
			strings.HasPrefix(id, "babbage") ||
			strings.HasPrefix(id, "sora") ||
			strings.HasPrefix(id, "gpt-image") ||
			strings.Contains(id, "-tts") ||
			strings.Contains(id, "-transcribe") ||
			strings.Contains(id, "-realtime") ||
			strings.Contains(id, "computer-use") ||
			strings.HasSuffix(id, "-instruct") {
			continue
		}

		models = append(models, provider.ModelInfo{
			ID:          id,
			Name:        id,
			DisplayName: id,
		})
	}

	slices.SortFunc(models, func(a, b provider.ModelInfo) int { return cmp.Compare(a.ID, b.ID) })

	return models, nil
}

// openaiReasoningEffort maps ThinkingLevel to OpenAI's ReasoningEffort.
// Returns empty string for ThinkingOff (no change to default).
func openaiReasoningEffort(level provider.ThinkingLevel) shared.ReasoningEffort {
	switch level {
	case provider.ThinkingNormal:
		return shared.ReasoningEffortMedium
	case provider.ThinkingHigh:
		return shared.ReasoningEffortHigh
	case provider.ThinkingUltra:
		return shared.ReasoningEffortXhigh
	default:
		return ""
	}
}

// Ensure Client implements Provider
var _ provider.Provider = (*Client)(nil)

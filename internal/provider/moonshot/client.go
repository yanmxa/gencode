// Package moonshot implements the LLMProvider interface using the Moonshot AI platform.
// Moonshot's API is OpenAI-compatible, so we reuse the openai-go SDK with a custom base URL.
package moonshot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/streamutil"
)

// Client implements the LLMProvider interface for Moonshot AI using the OpenAI SDK.
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

// Stream sends a completion request and returns a channel of streaming chunks.
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk)

	go func() {
		defer close(ch)

		// Convert messages to OpenAI format
		messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(opts.Messages)+1)

		// Add system prompt if provided
		if opts.SystemPrompt != "" {
			messages = append(messages, openai.SystemMessage(opts.SystemPrompt))
		}

		for _, msg := range opts.Messages {
			switch msg.Role {
			case message.RoleUser:
				if msg.ToolResult != nil {
					messages = append(messages, openai.ToolMessage(
						msg.ToolResult.Content,
						msg.ToolResult.ToolCallID,
					))
				} else if len(msg.Images) > 0 {
					// Multimodal message with images
					parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Images)+1)
					for _, img := range msg.Images {
						dataURI := fmt.Sprintf("data:%s;base64,%s", img.MediaType, img.Data)
						parts = append(parts, openai.ChatCompletionContentPartUnionParam{
							OfImageURL: &openai.ChatCompletionContentPartImageParam{
								ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
									URL: dataURI,
								},
							},
						})
					}
					if msg.Content != "" {
						parts = append(parts, openai.ChatCompletionContentPartUnionParam{
							OfText: &openai.ChatCompletionContentPartTextParam{
								Text: msg.Content,
							},
						})
					}
					messages = append(messages, openai.ChatCompletionMessageParamUnion{
						OfUser: &openai.ChatCompletionUserMessageParam{
							Content: openai.ChatCompletionUserMessageParamContentUnion{
								OfArrayOfContentParts: parts,
							},
						},
					})
				} else {
					messages = append(messages, openai.UserMessage(msg.Content))
				}
			case message.RoleAssistant:
				var asstMsg openai.ChatCompletionAssistantMessageParam
				if msg.Content != "" {
					asstMsg.Content.OfString = openai.Opt(msg.Content)
				}
				if len(msg.ToolCalls) > 0 {
					asstMsg.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
					for i, tc := range msg.ToolCalls {
						asstMsg.ToolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID: tc.ID,
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
									Name:      tc.Name,
									Arguments: tc.Input,
								},
							},
						}
					}
				}
				// Use saved thinking content if available, otherwise empty string
				// Moonshot requires reasoning_content for all assistant messages when thinking is enabled
				asstMsg.SetExtraFields(map[string]any{"reasoning_content": msg.Thinking})
				messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asstMsg})
			default: // system messages
				messages = append(messages, openai.SystemMessage(msg.Content))
			}
		}

		// Build request params
		params := openai.ChatCompletionNewParams{
			Model:    opts.Model,
			Messages: messages,
		}

		// Enable thinking mode for Kimi thinking models
		// The reasoning_content will be captured and replayed in multi-turn conversations
		params.SetExtraFields(map[string]any{
			"thinking": map[string]any{"type": "enabled"},
		})

		if opts.MaxTokens > 0 {
			params.MaxCompletionTokens = openai.Int(int64(opts.MaxTokens))
		}

		if opts.Temperature > 0 {
			params.Temperature = openai.Float(opts.Temperature)
		}

		// Add tools if provided
		if len(opts.Tools) > 0 {
			tools := make([]openai.ChatCompletionToolUnionParam, 0, len(opts.Tools))
			for _, t := range opts.Tools {
				var funcParams openai.FunctionParameters
				if props, ok := t.Parameters.(map[string]any); ok {
					funcParams = props
				}

				tools = append(tools, openai.ChatCompletionToolUnionParam{
					OfFunction: &openai.ChatCompletionFunctionToolParam{
						Function: openai.FunctionDefinitionParam{
							Name:        t.Name,
							Description: openai.String(t.Description),
							Parameters:  funcParams,
						},
					},
				})
			}
			params.Tools = tools
		}

		// Log request
		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Chat.Completions.NewStreaming(ctx, params)

		state := streamutil.NewState(c.name)

		// Track tool calls
		toolCalls := make(map[int]*message.ToolCall)

		// Read stream events
		for stream.Next() {
			chunk := stream.Current()
			state.Count()

			for _, choice := range chunk.Choices {
				// Handle reasoning_content (thinking) for Kimi thinking models
				// Parse the raw JSON to extract reasoning_content since it's not in the SDK struct
				rawJSON := choice.Delta.RawJSON()
				if rawJSON != "" {
					var deltaMap map[string]any
					if err := json.Unmarshal([]byte(rawJSON), &deltaMap); err == nil {
						if rc, ok := deltaMap["reasoning_content"]; ok && rc != nil {
							if content, ok := rc.(string); ok && content != "" {
								state.EmitThinking(ch, content)
							}
						}
					}
				}

				// Handle text delta
				if choice.Delta.Content != "" {
					state.EmitText(ch, choice.Delta.Content)
				}

				// Handle tool calls
				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)

					if _, exists := toolCalls[idx]; !exists {
						toolCalls[idx] = &message.ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
						}
						state.EmitToolStart(ch, tc.ID, tc.Function.Name)
					}

					if tc.Function.Arguments != "" {
						toolCalls[idx].Input += tc.Function.Arguments
						state.EmitToolInput(ch, toolCalls[idx].ID, tc.Function.Arguments)
					}
				}

				// Handle finish reason
				if choice.FinishReason != "" {
					switch choice.FinishReason {
					case "stop":
						state.Response.StopReason = "end_turn"
					case "tool_calls":
						state.Response.StopReason = "tool_use"
					case "length":
						state.Response.StopReason = "max_tokens"
					default:
						state.Response.StopReason = choice.FinishReason
					}
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
		state.Finish(ctx, ch)
	}()

	return ch
}

// staticModels is the fallback list when the models API is unavailable.
var staticModels = []provider.ModelInfo{
	{ID: "moonshot-v1-auto", Name: "moonshot-v1-auto", DisplayName: "Moonshot V1 Auto"},
	{ID: "moonshot-v1-128k", Name: "moonshot-v1-128k", DisplayName: "Moonshot V1 128K"},
	{ID: "kimi-k2-0711-preview", Name: "kimi-k2-0711-preview", DisplayName: "Kimi K2 0711 Preview"},
	{ID: "kimi-k2-0905-preview", Name: "kimi-k2-0905-preview", DisplayName: "Kimi K2 0905 Preview"},
}

// ListModels returns the available models for Moonshot AI using the API.
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	page, err := c.client.Models.List(ctx)
	if err != nil {
		// Fall back to static models if API call fails
		return staticModels, err
	}

	models := make([]provider.ModelInfo, 0)
	for _, m := range page.Data {
		id := m.ID
		info := provider.ModelInfo{
			ID:          id,
			Name:        id,
			DisplayName: id,
		}
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

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models, nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

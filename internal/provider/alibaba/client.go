// Package alibaba implements the LLMProvider interface using the Alibaba Cloud DashScope platform.
// DashScope's API is OpenAI-compatible, so we reuse the openai-go SDK with a custom base URL.
package alibaba

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// Client implements the LLMProvider interface for Qwen using the OpenAI SDK.
type Client struct {
	client openai.Client
	name   string
}

// NewClient creates a new Qwen client with the given OpenAI SDK client.
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

// isThinkingModel returns true if the model supports thinking/reasoning mode.
func isThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "qwq") || strings.Contains(lower, "thinking")
}

// extractReasoningContent parses reasoning_content from a raw JSON delta.
func extractReasoningContent(rawJSON string) string {
	if rawJSON == "" {
		return ""
	}
	var delta map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &delta); err != nil {
		return ""
	}
	if rc, ok := delta["reasoning_content"].(string); ok {
		return rc
	}
	return ""
}

// Stream sends a completion request and returns a channel of streaming chunks.
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk)

	go func() {
		defer close(ch)

		thinking := isThinkingModel(opts.Model)

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
				if thinking {
					asstMsg.SetExtraFields(map[string]any{"reasoning_content": msg.Thinking})
				}
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

		if thinking {
			params.SetExtraFields(map[string]any{"enable_thinking": true})
		}

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

		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		stream := c.client.Chat.Completions.NewStreaming(ctx, params)

		toolCalls := make(map[int]*message.ToolCall)
		var response message.CompletionResponse

		streamStart := time.Now()
		chunkCount := 0

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			for _, choice := range chunk.Choices {
				// Handle reasoning_content for thinking models
				if thinking {
					if content := extractReasoningContent(choice.Delta.RawJSON()); content != "" {
						ch <- message.StreamChunk{
							Type: message.ChunkTypeThinking,
							Text: content,
						}
						response.Thinking += content
					}
				}

				// Handle text delta
				if choice.Delta.Content != "" {
					ch <- message.StreamChunk{
						Type: message.ChunkTypeText,
						Text: choice.Delta.Content,
					}
					response.Content += choice.Delta.Content
				}

				// Handle tool calls
				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)

					if _, exists := toolCalls[idx]; !exists {
						toolCalls[idx] = &message.ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
						}
						ch <- message.StreamChunk{
							Type:     message.ChunkTypeToolStart,
							ToolID:   tc.ID,
							ToolName: tc.Function.Name,
						}
					}

					if tc.Function.Arguments != "" {
						toolCalls[idx].Input += tc.Function.Arguments
						ch <- message.StreamChunk{
							Type:   message.ChunkTypeToolInput,
							ToolID: toolCalls[idx].ID,
							Text:   tc.Function.Arguments,
						}
					}
				}

				// Handle finish reason
				if choice.FinishReason != "" {
					switch choice.FinishReason {
					case "stop":
						response.StopReason = "end_turn"
					case "tool_calls":
						response.StopReason = "tool_use"
					case "length":
						response.StopReason = "max_tokens"
					default:
						response.StopReason = choice.FinishReason
					}
				}
			}

			if chunk.Usage.PromptTokens > 0 {
				response.Usage.InputTokens = int(chunk.Usage.PromptTokens)
			}
			if chunk.Usage.CompletionTokens > 0 {
				response.Usage.OutputTokens = int(chunk.Usage.CompletionTokens)
			}
		}

		log.LogStreamDone(c.name, time.Since(streamStart), chunkCount)

		if err := stream.Err(); err != nil {
			log.LogError(c.name, err)
			ch <- message.StreamChunk{
				Type:  message.ChunkTypeError,
				Error: err,
			}
			return
		}

		for _, tc := range toolCalls {
			response.ToolCalls = append(response.ToolCalls, *tc)
		}

		log.LogResponseCtx(ctx, c.name, response)

		ch <- message.StreamChunk{
			Type:     message.ChunkTypeDone,
			Response: &response,
		}
	}()

	return ch
}

// staticModels is the fallback list when the models API is unavailable.
var staticModels = []provider.ModelInfo{
	{ID: "qwen3-235b-a22b", Name: "qwen3-235b-a22b", DisplayName: "Qwen3 235B A22B"},
	{ID: "qwen3-30b-a3b", Name: "qwen3-30b-a3b", DisplayName: "Qwen3 30B A3B"},
	{ID: "qwen3-32b", Name: "qwen3-32b", DisplayName: "Qwen3 32B"},
	{ID: "qwen3-14b", Name: "qwen3-14b", DisplayName: "Qwen3 14B"},
	{ID: "qwen-plus", Name: "qwen-plus", DisplayName: "Qwen Plus"},
	{ID: "qwen-max", Name: "qwen-max", DisplayName: "Qwen Max"},
	{ID: "qwen-turbo", Name: "qwen-turbo", DisplayName: "Qwen Turbo"},
	{ID: "qwq-plus", Name: "qwq-plus", DisplayName: "QwQ Plus (Thinking)"},
	{ID: "qwen-coder-plus", Name: "qwen-coder-plus", DisplayName: "Qwen Coder Plus"},
}

// ListModels returns the available models for Qwen using the API.
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	page, err := c.client.Models.List(ctx)
	if err != nil {
		return staticModels, err
	}

	models := make([]provider.ModelInfo, 0)
	for _, m := range page.Data {
		models = append(models, provider.ModelInfo{
			ID:          m.ID,
			Name:        m.ID,
			DisplayName: m.ID,
		})
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

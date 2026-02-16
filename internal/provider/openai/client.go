package openai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// Client implements the LLMProvider interface using the OpenAI SDK
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
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	if isResponsesModel(opts.Model) {
		return c.streamResponses(ctx, opts)
	}
	return c.streamChatCompletions(ctx, opts)
}

// streamResponses implements streaming via the Responses API for codex models.
func (c *Client) streamResponses(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk)

	go func() {
		defer close(ch)

		// Convert messages to Responses API input items
		var inputItems responses.ResponseInputParam

		for _, msg := range opts.Messages {
			switch msg.Role {
			case message.RoleUser:
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
			case message.RoleAssistant:
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
		log.LogRequest(c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Responses.NewStreaming(ctx, params)

		// Track tool calls by item ID
		toolCalls := make(map[string]*message.ToolCall)
		var response message.CompletionResponse
		hasToolCalls := false

		// Stream timing and counting
		streamStart := time.Now()
		chunkCount := 0

		// Read stream events
		for stream.Next() {
			event := stream.Current()
			chunkCount++

			switch event.Type {
			case "response.output_text.delta":
				delta := event.AsResponseOutputTextDelta()
				ch <- message.StreamChunk{
					Type: message.ChunkTypeText,
					Text: delta.Delta,
				}
				response.Content += delta.Delta

			case "response.output_item.added":
				itemEvent := event.AsResponseOutputItemAdded()
				if itemEvent.Item.Type == "function_call" {
					funcCall := itemEvent.Item.AsFunctionCall()
					hasToolCalls = true
					toolCalls[funcCall.ID] = &message.ToolCall{
						ID:   funcCall.CallID,
						Name: funcCall.Name,
					}
					ch <- message.StreamChunk{
						Type:     message.ChunkTypeToolStart,
						ToolID:   funcCall.CallID,
						ToolName: funcCall.Name,
					}
				}

			case "response.function_call_arguments.delta":
				delta := event.AsResponseFunctionCallArgumentsDelta()
				if tc, ok := toolCalls[delta.ItemID]; ok {
					tc.Input += delta.Delta
					ch <- message.StreamChunk{
						Type:   message.ChunkTypeToolInput,
						ToolID: tc.ID,
						Text:   delta.Delta,
					}
				}

			case "response.completed":
				completed := event.AsResponseCompleted()
				resp := completed.Response

				// Map usage
				response.Usage.InputTokens = int(resp.Usage.InputTokens)
				response.Usage.OutputTokens = int(resp.Usage.OutputTokens)

				// Determine stop reason
				switch resp.Status {
				case responses.ResponseStatusCompleted:
					if hasToolCalls {
						response.StopReason = "tool_use"
					} else {
						response.StopReason = "end_turn"
					}
				case responses.ResponseStatusIncomplete:
					response.StopReason = "max_tokens"
				default:
					response.StopReason = string(resp.Status)
				}

			case "error":
				errEvent := event.AsError()
				log.LogError(c.name, fmt.Errorf("responses API error: %s", errEvent.Message))
				ch <- message.StreamChunk{
					Type:  message.ChunkTypeError,
					Error: fmt.Errorf("responses API error: %s", errEvent.Message),
				}
				return
			}
		}

		// Log stream done
		log.LogStreamDone(c.name, time.Since(streamStart), chunkCount)

		if err := stream.Err(); err != nil {
			log.LogError(c.name, err)
			ch <- message.StreamChunk{
				Type:  message.ChunkTypeError,
				Error: err,
			}
			return
		}

		// Collect tool calls
		for _, tc := range toolCalls {
			response.ToolCalls = append(response.ToolCalls, *tc)
		}

		// Log response
		log.LogResponse(c.name, response)

		ch <- message.StreamChunk{
			Type:     message.ChunkTypeDone,
			Response: &response,
		}
	}()

	return ch
}

// streamChatCompletions implements streaming via the Chat Completions API.
func (c *Client) streamChatCompletions(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
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
					// Tool result message
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
				if len(msg.ToolCalls) > 0 {
					// Assistant message with tool calls
					var asstMsg openai.ChatCompletionAssistantMessageParam
					if msg.Content != "" {
						asstMsg.Content.OfString = openai.Opt(msg.Content)
					}
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
					messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asstMsg})
				} else {
					messages = append(messages, openai.AssistantMessage(msg.Content))
				}
			default: // system messages
				messages = append(messages, openai.SystemMessage(msg.Content))
			}
		}

		// Build request params
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

		// Add tools if provided
		if len(opts.Tools) > 0 {
			tools := make([]openai.ChatCompletionToolUnionParam, 0, len(opts.Tools))
			for _, t := range opts.Tools {
				// Convert parameters to FunctionParameters
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
		log.LogRequest(c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Chat.Completions.NewStreaming(ctx, params)

		// Track tool calls
		toolCalls := make(map[int]*message.ToolCall)
		var response message.CompletionResponse

		// Stream timing and counting
		streamStart := time.Now()
		chunkCount := 0

		// Read stream events
		for stream.Next() {
			chunk := stream.Current()
			chunkCount++

			for _, choice := range chunk.Choices {
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

					// Initialize new tool call
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

					// Accumulate arguments
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

			// Handle usage
			if chunk.Usage.PromptTokens > 0 {
				response.Usage.InputTokens = int(chunk.Usage.PromptTokens)
			}
			if chunk.Usage.CompletionTokens > 0 {
				response.Usage.OutputTokens = int(chunk.Usage.CompletionTokens)
			}
		}

		// Log stream done
		log.LogStreamDone(c.name, time.Since(streamStart), chunkCount)

		if err := stream.Err(); err != nil {
			log.LogError(c.name, err)
			ch <- message.StreamChunk{
				Type:  message.ChunkTypeError,
				Error: err,
			}
			return
		}

		// Collect tool calls
		for _, tc := range toolCalls {
			response.ToolCalls = append(response.ToolCalls, *tc)
		}

		// Log response
		log.LogResponse(c.name, response)

		ch <- message.StreamChunk{
			Type:     message.ChunkTypeDone,
			Response: &response,
		}
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

	models := make([]provider.ModelInfo, 0)

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

	// Sort models by ID for consistent ordering
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models, nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

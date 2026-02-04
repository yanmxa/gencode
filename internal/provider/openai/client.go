package openai

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go"

	"github.com/yanmxa/gencode/internal/log"
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

// Stream sends a completion request and returns a channel of streaming chunks
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk)

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
			case "user":
				if msg.ToolResult != nil {
					// Tool result message
					messages = append(messages, openai.ToolMessage(
						msg.ToolResult.Content,
						msg.ToolResult.ToolCallID,
					))
				} else {
					messages = append(messages, openai.UserMessage(msg.Content))
				}
			case "assistant":
				if len(msg.ToolCalls) > 0 {
					// Assistant message with tool calls
					var asstMsg openai.ChatCompletionAssistantMessageParam
					if msg.Content != "" {
						asstMsg.Content.OfString = openai.Opt(msg.Content)
					}
					asstMsg.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
					for i, tc := range msg.ToolCalls {
						asstMsg.ToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Input,
							},
						}
					}
					messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asstMsg})
				} else {
					messages = append(messages, openai.AssistantMessage(msg.Content))
				}
			case "system":
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
			tools := make([]openai.ChatCompletionToolParam, 0, len(opts.Tools))
			for _, t := range opts.Tools {
				// Convert parameters to FunctionParameters
				var funcParams openai.FunctionParameters
				if props, ok := t.Parameters.(map[string]any); ok {
					funcParams = props
				}

				tools = append(tools, openai.ChatCompletionToolParam{
					Function: openai.FunctionDefinitionParam{
						Name:        t.Name,
						Description: openai.String(t.Description),
						Parameters:  funcParams,
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
		toolCalls := make(map[int]*provider.ToolCall)
		var response provider.CompletionResponse

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
					ch <- provider.StreamChunk{
						Type: provider.ChunkTypeText,
						Text: choice.Delta.Content,
					}
					response.Content += choice.Delta.Content
				}

				// Handle tool calls
				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)

					// Initialize new tool call
					if _, exists := toolCalls[idx]; !exists {
						toolCalls[idx] = &provider.ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
						}
						ch <- provider.StreamChunk{
							Type:     provider.ChunkTypeToolStart,
							ToolID:   tc.ID,
							ToolName: tc.Function.Name,
						}
					}

					// Accumulate arguments
					if tc.Function.Arguments != "" {
						toolCalls[idx].Input += tc.Function.Arguments
						ch <- provider.StreamChunk{
							Type:   provider.ChunkTypeToolInput,
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
			ch <- provider.StreamChunk{
				Type:  provider.ChunkTypeError,
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

		ch <- provider.StreamChunk{
			Type:     provider.ChunkTypeDone,
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
		// Filter for chat-related models (gpt-*, o1*, o3*)
		id := m.ID
		if strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "o1") || strings.HasPrefix(id, "o3") {
			// Skip deprecated/snapshot models for cleaner display
			if strings.Contains(id, "-preview") || strings.Contains(id, "-2024") || strings.Contains(id, "-2025") {
				continue
			}

			models = append(models, provider.ModelInfo{
				ID:          id,
				Name:        id,
				DisplayName: id,
			})
		}
	}

	// Sort models by ID for consistent ordering
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models, nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

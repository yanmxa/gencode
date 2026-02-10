package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/provider"
)

// Client implements the LLMProvider interface using the Anthropic SDK
type Client struct {
	client       anthropic.Client
	name         string
	cachedModels []provider.ModelInfo
}

// NewClient creates a new Anthropic client with the given SDK client
func NewClient(client anthropic.Client, name string) *Client {
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

		// Convert messages to Anthropic format
		anthropicMsgs := make([]anthropic.MessageParam, 0, len(opts.Messages))
		for _, msg := range opts.Messages {
			switch msg.Role {
			case "user":
				if msg.ToolResult != nil {
					// Tool result message
					anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(
						anthropic.NewToolResultBlock(
							msg.ToolResult.ToolCallID,
							msg.ToolResult.Content,
							msg.ToolResult.IsError,
						),
					))
				} else if len(msg.ContentParts) > 0 {
					// Multimodal message with images
					blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ContentParts))
					for _, part := range msg.ContentParts {
						switch part.Type {
						case provider.ContentTypeImage:
							if part.Image != nil {
								blocks = append(blocks, anthropic.NewImageBlockBase64(
									part.Image.MediaType,
									part.Image.Data,
								))
							}
						case provider.ContentTypeText:
							if part.Text != "" {
								blocks = append(blocks, anthropic.NewTextBlock(part.Text))
							}
						}
					}
					anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(blocks...))
				} else {
					anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(
						anthropic.NewTextBlock(msg.Content),
					))
				}
			case "assistant":
				if len(msg.ToolCalls) > 0 {
					// Assistant message with tool calls
					blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
					if msg.Content != "" {
						blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
					}
					for _, tc := range msg.ToolCalls {
						// Parse the JSON input string to any type
						var input any
						if tc.Input != "" {
							if err := json.Unmarshal([]byte(tc.Input), &input); err != nil {
								input = tc.Input // fallback to string if parse fails
							}
						} else {
							// For tools with no parameters, use empty object instead of nil
							input = map[string]any{}
						}
						blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
					}
					anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(blocks...))
				} else {
					anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(
						anthropic.NewTextBlock(msg.Content),
					))
				}
			}
		}

		// Build request params
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(opts.Model),
			MaxTokens: int64(opts.MaxTokens),
			Messages:  anthropicMsgs,
		}

		if opts.SystemPrompt != "" {
			params.System = []anthropic.TextBlockParam{
				{Text: opts.SystemPrompt},
			}
		}

		// Add tools if provided
		if len(opts.Tools) > 0 {
			tools := make([]anthropic.ToolUnionParam, 0, len(opts.Tools))
			for _, t := range opts.Tools {
				// Convert parameters to ToolInputSchemaParam
				inputSchema := anthropic.ToolInputSchemaParam{}
				if props, ok := t.Parameters.(map[string]any); ok {
					if properties, ok := props["properties"]; ok {
						inputSchema.Properties = properties
					}
					if required, ok := props["required"].([]string); ok {
						inputSchema.Required = required
					} else if required, ok := props["required"].([]any); ok {
						// Convert []any to []string
						requiredStrs := make([]string, 0, len(required))
						for _, r := range required {
							if s, ok := r.(string); ok {
								requiredStrs = append(requiredStrs, s)
							}
						}
						inputSchema.Required = requiredStrs
					}
				}

				tools = append(tools, anthropic.ToolUnionParam{
					OfTool: &anthropic.ToolParam{
						Name:        t.Name,
						Description: anthropic.String(t.Description),
						InputSchema: inputSchema,
					},
				})
			}
			params.Tools = tools
		}

		// Log request
		log.LogRequest(c.name, opts.Model, opts)

		// Create streaming request
		stream := c.client.Messages.NewStreaming(ctx, params)

		// Track tool calls
		var currentToolID string
		var currentToolName string
		var currentToolInput string
		var response provider.CompletionResponse

		// Stream timing and counting
		streamStart := time.Now()
		chunkCount := 0

		// Read stream events
		for stream.Next() {
			event := stream.Current()
			chunkCount++

			switch event.Type {
			case "content_block_start":
				block := event.AsContentBlockStart()
				if block.ContentBlock.Type == "tool_use" {
					currentToolID = block.ContentBlock.ID
					currentToolName = block.ContentBlock.Name
					currentToolInput = ""
					ch <- provider.StreamChunk{
						Type:     provider.ChunkTypeToolStart,
						ToolID:   currentToolID,
						ToolName: currentToolName,
					}
				}

			case "content_block_delta":
				delta := event.AsContentBlockDelta()
				switch delta.Delta.Type {
				case "text_delta":
					if delta.Delta.Text != "" {
						ch <- provider.StreamChunk{
							Type: provider.ChunkTypeText,
							Text: delta.Delta.Text,
						}
						response.Content += delta.Delta.Text
					}
				case "input_json_delta":
					if delta.Delta.PartialJSON != "" {
						ch <- provider.StreamChunk{
							Type:   provider.ChunkTypeToolInput,
							ToolID: currentToolID,
							Text:   delta.Delta.PartialJSON,
						}
						currentToolInput += delta.Delta.PartialJSON
					}
				}

			case "content_block_stop":
				// When a tool block ends, add the accumulated tool call
				if currentToolID != "" && currentToolName != "" {
					response.ToolCalls = append(response.ToolCalls, provider.ToolCall{
						ID:    currentToolID,
						Name:  currentToolName,
						Input: currentToolInput,
					})
					currentToolID = ""
					currentToolName = ""
					currentToolInput = ""
				}

			case "message_delta":
				msgDelta := event.AsMessageDelta()
				response.StopReason = string(msgDelta.Delta.StopReason)
				response.Usage.OutputTokens = int(msgDelta.Usage.OutputTokens)

			case "message_start":
				msgStart := event.AsMessageStart()
				response.Usage.InputTokens = int(msgStart.Message.Usage.InputTokens)
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

		// Log response
		log.LogResponse(c.name, response)

		ch <- provider.StreamChunk{
			Type:     provider.ChunkTypeDone,
			Response: &response,
		}
	}()

	return ch
}

// defaultModels is the fallback static model list
var defaultModels = []provider.ModelInfo{
	{ID: "claude-opus-4-5@20251101", Name: "Claude Opus 4.5", DisplayName: "Claude Opus 4.5 (Most Capable)"},
	{ID: "claude-sonnet-4-5@20250929", Name: "Claude Sonnet 4.5", DisplayName: "Claude Sonnet 4.5 (Balanced)"},
	{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", DisplayName: "Claude Sonnet 4"},
	{ID: "claude-haiku-3-5@20241022", Name: "Claude Haiku 3.5", DisplayName: "Claude Haiku 3.5 (Fast)"},
}

// ListModels returns available models using the Anthropic Models API,
// falling back to a static list if the API call fails.
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if len(c.cachedModels) > 0 {
		return c.cachedModels, nil
	}

	models, err := c.fetchModels(ctx)
	if err != nil {
		// Fall back to static model list
		c.cachedModels = defaultModels
		return c.cachedModels, nil
	}
	c.cachedModels = models
	return c.cachedModels, nil
}

// fetchModels fetches available models from the Anthropic Models API
func (c *Client) fetchModels(ctx context.Context) ([]provider.ModelInfo, error) {
	pager := c.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})

	var models []provider.ModelInfo
	for pager.Next() {
		m := pager.Current()
		models = append(models, provider.ModelInfo{
			ID:          m.ID,
			Name:        m.DisplayName,
			DisplayName: m.DisplayName,
		})
	}
	if err := pager.Err(); err != nil {
		return nil, err
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned from API")
	}
	return models, nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

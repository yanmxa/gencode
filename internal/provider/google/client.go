package google

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"google.golang.org/genai"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/provider"
)

// Client implements the LLMProvider interface using the Google GenAI SDK
type Client struct {
	client *genai.Client
	name   string
}

// NewClient creates a new Google client with the given SDK client
func NewClient(client *genai.Client, name string) *Client {
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

		// Convert messages to Google format
		contents := make([]*genai.Content, 0, len(opts.Messages))
		for _, msg := range opts.Messages {
			var role string
			switch msg.Role {
			case "user":
				role = "user"
			case "assistant":
				role = "model"
			default:
				role = msg.Role
			}

			parts := make([]*genai.Part, 0)

			if msg.ToolResult != nil {
				// Tool result as function response
				var result map[string]any
				if err := json.Unmarshal([]byte(msg.ToolResult.Content), &result); err != nil {
					// Wrap non-JSON content in a result object
					result = map[string]any{"result": msg.ToolResult.Content}
				}
				parts = append(parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						ID:       msg.ToolResult.ToolCallID,
						Name:     msg.ToolResult.ToolName,
						Response: result,
					},
				})
			} else if len(msg.ToolCalls) > 0 {
				// Tool calls as function calls
				if msg.Content != "" {
					parts = append(parts, &genai.Part{Text: msg.Content})
				}
				for _, tc := range msg.ToolCalls {
					var args map[string]any
					if tc.Input != "" {
						if err := json.Unmarshal([]byte(tc.Input), &args); err != nil {
							args = nil
						}
					}
					parts = append(parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							ID:   tc.ID,
							Name: tc.Name,
							Args: args,
						},
					})
				}
			} else {
				parts = append(parts, &genai.Part{Text: msg.Content})
			}

			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: parts,
			})
		}

		// Build config
		config := &genai.GenerateContentConfig{}

		if opts.SystemPrompt != "" {
			config.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{{Text: opts.SystemPrompt}},
			}
		}

		if opts.MaxTokens > 0 {
			config.MaxOutputTokens = int32(opts.MaxTokens)
		}

		if opts.Temperature > 0 {
			temp := float32(opts.Temperature)
			config.Temperature = &temp
		}

		// Add tools if provided
		if len(opts.Tools) > 0 {
			funcDecls := make([]*genai.FunctionDeclaration, 0, len(opts.Tools))
			for _, t := range opts.Tools {
				fd := &genai.FunctionDeclaration{
					Name:        t.Name,
					Description: t.Description,
				}
				// Use ParametersJsonSchema for JSON schema parameters
				if t.Parameters != nil {
					fd.ParametersJsonSchema = t.Parameters
				}
				funcDecls = append(funcDecls, fd)
			}
			config.Tools = []*genai.Tool{
				{FunctionDeclarations: funcDecls},
			}
		}

		// Log request
		log.LogRequest(c.name, opts.Model, opts)

		// Create streaming request
		var response provider.CompletionResponse

		for result, err := range c.client.Models.GenerateContentStream(ctx, opts.Model, contents, config) {
			if err != nil {
				log.LogError(c.name, err)
				ch <- provider.StreamChunk{
					Type:  provider.ChunkTypeError,
					Error: err,
				}
				return
			}

			// Process candidates
			for _, candidate := range result.Candidates {
				if candidate.Content == nil {
					continue
				}

				for _, part := range candidate.Content.Parts {
					// Handle text
					if part.Text != "" {
						ch <- provider.StreamChunk{
							Type: provider.ChunkTypeText,
							Text: part.Text,
						}
						response.Content += part.Text
					}

					// Handle function calls
					if part.FunctionCall != nil {
						fc := part.FunctionCall
						argsJSON, _ := json.Marshal(fc.Args)

						ch <- provider.StreamChunk{
							Type:     provider.ChunkTypeToolStart,
							ToolID:   fc.ID,
							ToolName: fc.Name,
						}

						ch <- provider.StreamChunk{
							Type:   provider.ChunkTypeToolInput,
							ToolID: fc.ID,
							Text:   string(argsJSON),
						}

						response.ToolCalls = append(response.ToolCalls, provider.ToolCall{
							ID:    fc.ID,
							Name:  fc.Name,
							Input: string(argsJSON),
						})
					}
				}

				// Handle finish reason
				if candidate.FinishReason != "" {
					switch candidate.FinishReason {
					case "STOP":
						response.StopReason = "end_turn"
					case "MAX_TOKENS":
						response.StopReason = "max_tokens"
					default:
						response.StopReason = string(candidate.FinishReason)
					}
				}
			}

			// Handle usage
			if result.UsageMetadata != nil {
				response.Usage.InputTokens = int(result.UsageMetadata.PromptTokenCount)
				response.Usage.OutputTokens = int(result.UsageMetadata.CandidatesTokenCount)
			}
		}

		// Check for tool calls stop reason
		if len(response.ToolCalls) > 0 && response.StopReason == "" {
			response.StopReason = "tool_use"
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

// ListModels returns the available models for Google using the API
func (c *Client) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	// Use Google API to dynamically fetch models
	models := make([]provider.ModelInfo, 0)

	for m, err := range c.client.Models.All(ctx) {
		if err != nil {
			return nil, err
		}

		// Filter for Gemini models
		name := m.Name
		if strings.Contains(name, "gemini") {
			// Extract short model ID from full name (e.g., "models/gemini-2.0-flash" -> "gemini-2.0-flash")
			id, _ := strings.CutPrefix(name, "models/")

			// Skip deprecated/experimental models for cleaner display
			if strings.Contains(id, "-exp") || strings.Contains(id, "-latest") {
				continue
			}

			displayName := m.DisplayName
			if displayName == "" {
				displayName = id
			}

			models = append(models, provider.ModelInfo{
				ID:          id,
				Name:        displayName,
				DisplayName: displayName,
			})
		}
	}

	// Sort models by ID for consistent ordering
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models, nil
}

// NewAPIKeyClient creates a new Google client using API Key authentication
func NewAPIKeyClient(ctx context.Context) (provider.LLMProvider, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return NewClient(client, "google:api_key"), nil
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

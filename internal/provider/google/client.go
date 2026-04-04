package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	stdlog "log"
	"os"
	"sort"
	"strings"

	"google.golang.org/genai"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/streamutil"
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
func (c *Client) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk)

	go func() {
		defer close(ch)

		// Convert messages to Google format
		contents := make([]*genai.Content, 0, len(opts.Messages))
		for _, msg := range opts.Messages {
			var role string
			switch msg.Role {
			case message.RoleUser:
				role = "user"
			case message.RoleAssistant:
				role = "model"
			default:
				role = string(msg.Role)
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
					p := &genai.Part{
						FunctionCall: &genai.FunctionCall{
							ID:   tc.ID,
							Name: tc.Name,
							Args: args,
						},
					}
					if len(tc.ThoughtSignature) > 0 {
						p.ThoughtSignature = tc.ThoughtSignature
					}
					parts = append(parts, p)
				}
			} else if len(msg.Images) > 0 {
				// Multimodal message with images
				for _, img := range msg.Images {
					decoded, err := base64.StdEncoding.DecodeString(img.Data)
					if err == nil {
						parts = append(parts, &genai.Part{
							InlineData: &genai.Blob{
								MIMEType: img.MediaType,
								Data:     decoded,
							},
						})
					}
				}
				if msg.Content != "" {
					parts = append(parts, &genai.Part{Text: msg.Content})
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

		// Configure thinking
		if opts.ThinkingLevel > provider.ThinkingOff {
			config.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  googleThinkingBudget(opts.ThinkingLevel),
			}
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
		log.LogRequestCtx(ctx, c.name, opts.Model, opts)

		state := streamutil.NewState(c.name)

		for result, err := range c.client.Models.GenerateContentStream(ctx, opts.Model, contents, config) {
			if err != nil {
				state.Fail(ch, err)
				return
			}
			state.Count()

			// Process candidates
			for _, candidate := range result.Candidates {
				if candidate.Content == nil {
					continue
				}

				for _, part := range candidate.Content.Parts {
					// Handle text (distinguish thinking from regular text)
					if part.Text != "" {
						if part.Thought {
							state.EmitThinking(ch, part.Text)
						} else {
							state.EmitText(ch, part.Text)
						}
					}

					// Handle function calls
					if part.FunctionCall != nil {
						fc := part.FunctionCall
						argsJSON, _ := json.Marshal(fc.Args)

						state.EmitToolStart(ch, fc.ID, fc.Name)
						state.EmitToolInput(ch, fc.ID, string(argsJSON))

						state.Response.ToolCalls = append(state.Response.ToolCalls, message.ToolCall{
							ID:               fc.ID,
							Name:             fc.Name,
							Input:            string(argsJSON),
							ThoughtSignature: part.ThoughtSignature,
						})
					}
				}

				// Handle finish reason
				if candidate.FinishReason != "" {
					switch candidate.FinishReason {
					case "STOP":
						state.Response.StopReason = "end_turn"
					case "MAX_TOKENS":
						state.Response.StopReason = "max_tokens"
					default:
						state.Response.StopReason = string(candidate.FinishReason)
					}
				}
			}

			// Handle usage
			if result.UsageMetadata != nil {
				state.UpdateUsage(int(result.UsageMetadata.PromptTokenCount), int(result.UsageMetadata.CandidatesTokenCount))
			}
		}

		state.EnsureToolUseStopReason()
		state.Finish(ctx, ch)
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
				ID:               id,
				Name:             displayName,
				DisplayName:      displayName,
				InputTokenLimit:  int(m.InputTokenLimit),
				OutputTokenLimit: int(m.OutputTokenLimit),
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

	// The Google GenAI SDK warns via log.Printf when both GOOGLE_API_KEY and
	// GEMINI_API_KEY are set. Suppress it to prevent leaking into the TUI.
	w := stdlog.Writer()
	stdlog.SetOutput(io.Discard)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	stdlog.SetOutput(w)
	if err != nil {
		return nil, err
	}

	return NewClient(client, "google:api_key"), nil
}

// googleThinkingBudget returns a pointer to the thinking budget for the given level.
func googleThinkingBudget(level provider.ThinkingLevel) *int32 {
	b := level.BudgetTokens()
	if b == 0 {
		return nil
	}
	budget := int32(b)
	return &budget
}

// Ensure Client implements LLMProvider
var _ provider.LLMProvider = (*Client)(nil)

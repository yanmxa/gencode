package provider

import (
	"context"

	"github.com/yanmxa/gencode/internal/message"
)

// Provider represents a provider name
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
	ProviderMoonshot  Provider = "moonshot"
	ProviderAlibaba   Provider = "alibaba"
)

// AuthMethod represents an authentication method
type AuthMethod string

const (
	AuthAPIKey  AuthMethod = "api_key"
	AuthVertex  AuthMethod = "vertex"
	AuthBedrock AuthMethod = "bedrock"
)

// ProviderMeta contains static metadata about a provider
type ProviderMeta struct {
	Provider    Provider
	AuthMethod  AuthMethod
	EnvVars     []string // Required environment variables
	DisplayName string
}

// Key returns a unique key for this provider configuration
func (m ProviderMeta) Key() string {
	return string(m.Provider) + ":" + string(m.AuthMethod)
}

// ThinkingLevel controls extended thinking / reasoning effort.
type ThinkingLevel int

const (
	ThinkingOff    ThinkingLevel = iota // No extended thinking
	ThinkingNormal                      // Default thinking (moderate budget)
	ThinkingHigh                        // Extended thinking (larger budget)
	ThinkingUltra                       // Maximum thinking budget
)

// String returns a human-readable label for the thinking level.
func (t ThinkingLevel) String() string {
	switch t {
	case ThinkingNormal:
		return "think"
	case ThinkingHigh:
		return "think+"
	case ThinkingUltra:
		return "ultrathink"
	default:
		return "off"
	}
}

// Next cycles to the next thinking level.
func (t ThinkingLevel) Next() ThinkingLevel {
	return (t + 1) % 4
}

// BudgetTokens returns the token budget for this thinking level.
// Returns 0 for ThinkingOff.
func (t ThinkingLevel) BudgetTokens() int {
	switch t {
	case ThinkingNormal:
		return 5000
	case ThinkingHigh:
		return 32000
	case ThinkingUltra:
		return 128000
	default:
		return 0
	}
}

// ModelInfo represents information about an available model
type ModelInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName,omitempty"`
	InputTokenLimit  int    `json:"inputTokenLimit,omitempty"`
	OutputTokenLimit int    `json:"outputTokenLimit,omitempty"`
}

// CompletionOptions contains options for a completion request
type CompletionOptions struct {
	Model         string
	Messages      []message.Message
	MaxTokens     int
	Temperature   float64
	Tools         []ToolSchema
	SystemPrompt  string
	ThinkingLevel ThinkingLevel
}

// ToolSchema is a backward-compatible alias for message.ToolSchema.
type ToolSchema = message.ToolSchema

// LLMProvider is the interface that all providers must implement
type LLMProvider interface {
	// Stream sends a completion request and returns a channel of streaming chunks
	Stream(ctx context.Context, opts CompletionOptions) <-chan message.StreamChunk

	// ListModels returns the available models for this provider
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// Name returns the provider name
	Name() string
}

// ModelLimitsFetcher is an optional interface for providers that can fetch
// token limits for a specific model via API (e.g. DashScope model detail endpoint).
type ModelLimitsFetcher interface {
	FetchModelLimits(ctx context.Context, modelID string) (inputLimit, outputLimit int, err error)
}

// ProviderFactory creates a new LLMProvider instance
type ProviderFactory func(ctx context.Context) (LLMProvider, error)

// Complete is a helper function that collects stream chunks into a complete response
// This provides non-streaming output from any LLMProvider
func Complete(ctx context.Context, provider LLMProvider, opts CompletionOptions) (message.CompletionResponse, error) {
	var response message.CompletionResponse

	streamChan := provider.Stream(ctx, opts)

	for chunk := range streamChan {
		switch chunk.Type {
		case message.ChunkTypeText:
			response.Content += chunk.Text
		case message.ChunkTypeToolStart, message.ChunkTypeToolInput:
			// Tool calls are accumulated in the done chunk
		case message.ChunkTypeDone:
			if chunk.Response != nil {
				return *chunk.Response, nil
			}
			return response, nil
		case message.ChunkTypeError:
			return response, chunk.Error
		}
	}

	return response, nil
}

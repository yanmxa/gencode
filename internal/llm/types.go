package llm

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/core"
)

// Name represents a provider name
type Name string

const (
	Anthropic Name = "anthropic"
	OpenAI    Name = "openai"
	Google    Name = "google"
	Moonshot  Name = "moonshot"
	Alibaba   Name = "alibaba"
)

// AuthMethod is an alias for core.AuthMethod so that existing consumers
// of the provider package continue to compile without changes.
type AuthMethod = core.AuthMethod

// Auth method constants re-exported from core.
const (
	AuthAPIKey  = core.AuthAPIKey
	AuthVertex  = core.AuthVertex
	AuthBedrock = core.AuthBedrock
)

// Meta contains static metadata about a provider
type Meta struct {
	Provider    Name
	AuthMethod  AuthMethod
	EnvVars     []string // Required environment variables
	DisplayName string
}

// Key returns a unique key for this provider configuration
func (m Meta) Key() string {
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
	Messages      []core.Message
	MaxTokens     int
	Temperature   float64
	Tools         []ToolSchema
	SystemPrompt  string
	ThinkingLevel ThinkingLevel
}

// ToolSchema is a backward-compatible alias for core.ToolSchema.
type ToolSchema = core.ToolSchema

// Provider is the interface that all providers must implement
type Provider interface {
	// Stream sends a completion request and returns a channel of streaming chunks
	Stream(ctx context.Context, opts CompletionOptions) <-chan core.StreamChunk

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

// Factory creates a new Provider instance
type Factory func(ctx context.Context) (Provider, error)

// Complete is a helper function that collects stream chunks into a complete response
// This provides non-streaming output from any Provider
func Complete(ctx context.Context, provider Provider, opts CompletionOptions) (core.CompletionResponse, error) {
	var response core.CompletionResponse

	streamChan := provider.Stream(ctx, opts)

	gotDone := false
	for chunk := range streamChan {
		switch chunk.Type {
		case core.ChunkTypeText:
			response.Content += chunk.Text
		case core.ChunkTypeToolStart, core.ChunkTypeToolInput:
			// Tool calls are accumulated in the done chunk
		case core.ChunkTypeDone:
			if chunk.Response != nil {
				return *chunk.Response, nil
			}
			gotDone = true
		case core.ChunkTypeError:
			return response, chunk.Error
		}
	}

	if !gotDone {
		return response, fmt.Errorf("stream closed without completion")
	}
	return response, nil
}

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
	Model       string
	Messages    []message.Message
	MaxTokens   int
	Temperature float64
	Tools       []Tool
	SystemPrompt string
}

// Tool represents a tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON Schema
}

// LLMProvider is the interface that all providers must implement
type LLMProvider interface {
	// Stream sends a completion request and returns a channel of streaming chunks
	Stream(ctx context.Context, opts CompletionOptions) <-chan message.StreamChunk

	// ListModels returns the available models for this provider
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// Name returns the provider name
	Name() string
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

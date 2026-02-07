package provider

import (
	"context"
)

// Provider represents a provider name
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
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
	Messages    []Message
	MaxTokens   int
	Temperature float64
	Tools       []Tool
	SystemPrompt string
}

// Message represents a chat message
type Message struct {
	Role       string       `json:"role"` // "user", "assistant", "system"
	Content    string       `json:"content,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolResult *ToolResult  `json:"tool_result,omitempty"`
}

// Tool represents a tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON Schema
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // JSON string
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name,omitempty"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// CompletionResponse represents a completion response
type CompletionResponse struct {
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	Usage      Usage      `json:"usage"`
}

// Usage contains token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ChunkType represents the type of a stream chunk
type ChunkType string

const (
	ChunkTypeText      ChunkType = "text"
	ChunkTypeToolStart ChunkType = "tool_start"
	ChunkTypeToolInput ChunkType = "tool_input"
	ChunkTypeDone      ChunkType = "done"
	ChunkTypeError     ChunkType = "error"
)

// StreamChunk represents a chunk in a streaming response
type StreamChunk struct {
	Type     ChunkType
	Text     string             // For text chunks
	ToolID   string             // For tool_start chunks
	ToolName string             // For tool_start chunks
	Response *CompletionResponse // For done chunks
	Error    error              // For error chunks
}

// LLMProvider is the interface that all providers must implement
type LLMProvider interface {
	// Stream sends a completion request and returns a channel of streaming chunks
	Stream(ctx context.Context, opts CompletionOptions) <-chan StreamChunk

	// ListModels returns the available models for this provider
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// Name returns the provider name
	Name() string
}

// ProviderFactory creates a new LLMProvider instance
type ProviderFactory func(ctx context.Context) (LLMProvider, error)

// Complete is a helper function that collects stream chunks into a complete response
// This provides non-streaming output from any LLMProvider
func Complete(ctx context.Context, provider LLMProvider, opts CompletionOptions) (CompletionResponse, error) {
	var response CompletionResponse

	streamChan := provider.Stream(ctx, opts)

	for chunk := range streamChan {
		switch chunk.Type {
		case ChunkTypeText:
			response.Content += chunk.Text
		case ChunkTypeToolStart, ChunkTypeToolInput:
			// Tool calls are accumulated in the done chunk
		case ChunkTypeDone:
			if chunk.Response != nil {
				return *chunk.Response, nil
			}
			return response, nil
		case ChunkTypeError:
			return response, chunk.Error
		}
	}

	return response, nil
}

package options

import (
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

const (
	DefaultMaxTokens    = 8192
	DefaultSystemPrompt = "You are a helpful AI coding assistant."
)

// DefaultModel returns the default model ID for a given provider and auth method.
func DefaultModel(providerName string, authMethod provider.AuthMethod) string {
	if providerName == "anthropic" && authMethod == provider.AuthVertex {
		return "claude-sonnet-4-5@20250929"
	}
	switch providerName {
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "openai":
		return "gpt-4o"
	case "google":
		return "gemini-2.0-flash"
	case "moonshot":
		return "moonshot-v1-auto"
	default:
		return "claude-sonnet-4-20250514"
	}
}

// NewCompletionOptions builds provider.CompletionOptions with default settings.
func NewCompletionOptions(model, prompt string) provider.CompletionOptions {
	return provider.CompletionOptions{
		Model:        model,
		MaxTokens:    DefaultMaxTokens,
		SystemPrompt: DefaultSystemPrompt,
		Messages:     []message.Message{message.UserMessage(prompt, nil)},
		Tools:        tool.GetToolSchemas(),
	}
}

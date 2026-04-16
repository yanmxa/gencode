// Package client wraps an LLM provider with model selection, token limits,
// and cumulative token usage tracking. It is the "configured LLM session" that
// binds a provider to a specific model and settings.
package client

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// defaultMaxTokens is the fallback max output tokens when neither the caller
// nor the provider specifies a limit. Inlined here (rather than referencing
// config.DefaultMaxTokens) to avoid pulling the config package — and its
// transitive deps — into this low-level client layer.
const defaultMaxTokens = 8192

// TokenUsage tracks token consumption for a conversation.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Client wraps an LLM provider with model selection, token limits,
// and cumulative token usage tracking.
type Client struct {
	Provider      provider.LLMProvider
	Model         string
	MaxTokens     int // custom override; 0 means resolve from provider
	ThinkingLevel provider.ThinkingLevel

	mu     sync.Mutex // protects tokens
	tokens TokenUsage
}

// NewClient creates a Client bound to the given provider and model.
func NewClient(p provider.LLMProvider, model string) *Client {
	return &Client{Provider: p, Model: model}
}

// AddUsage accumulates token usage from a completion response.
func (c *Client) AddUsage(usage message.Usage) {
	c.mu.Lock()
	c.tokens.InputTokens += usage.InputTokens
	c.tokens.OutputTokens += usage.OutputTokens
	c.tokens.TotalTokens = c.tokens.InputTokens + c.tokens.OutputTokens
	c.mu.Unlock()
}

// Tokens returns the accumulated token usage.
func (c *Client) Tokens() TokenUsage {
	c.mu.Lock()
	t := c.tokens
	c.mu.Unlock()
	return t
}

// Send sends a non-streaming completion request and returns the full response.
func (c *Client) Send(ctx context.Context, msgs []message.Message,
	tools []provider.ToolSchema, sysPrompt string,
) (message.CompletionResponse, error) {
	return provider.Complete(ctx, c.Provider, c.opts(msgs, tools, sysPrompt))
}

// Stream starts a streaming completion request and returns a chunk channel.
func (c *Client) Stream(ctx context.Context, msgs []message.Message,
	tools []provider.ToolSchema, sysPrompt string,
) <-chan message.StreamChunk {
	return c.Provider.Stream(ctx, c.opts(msgs, tools, sysPrompt))
}

// Complete sends a one-shot completion (custom max tokens, no tools).
// Used for utility calls like conversation compaction.
func (c *Client) Complete(ctx context.Context,
	sysPrompt string, msgs []message.Message, maxTokens int,
) (message.CompletionResponse, error) {
	return provider.Complete(ctx, c.Provider, provider.CompletionOptions{
		Model:        c.Model,
		SystemPrompt: sysPrompt,
		Messages:     msgs,
		MaxTokens:    maxTokens,
	})
}

// Name returns the provider name (e.g., "anthropic").
func (c *Client) Name() string {
	return c.Provider.Name()
}

// ModelID returns the model identifier.
func (c *Client) ModelID() string {
	return c.Model
}

// ResolveMaxTokens returns the effective output token limit.
// Priority: 1. Custom override (MaxTokens field)
//
//  2. Provider's model metadata (OutputTokenLimit from ListModels)
//  3. Default (8192)
func (c *Client) ResolveMaxTokens(ctx context.Context) int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	if limit := c.providerOutputLimit(ctx); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

// providerOutputLimit queries the provider's ListModels for the current model's
// output token limit. Returns 0 if not found.
func (c *Client) providerOutputLimit(ctx context.Context) int {
	if c.Provider == nil {
		return 0
	}
	models, err := c.Provider.ListModels(ctx)
	if err != nil {
		return 0
	}
	for _, m := range models {
		if m.ID == c.Model {
			return m.OutputTokenLimit
		}
	}
	return 0
}

// opts builds CompletionOptions from the client's configuration.
// It resolves the output token limit using the same priority as
// ResolveMaxTokens: custom override → provider metadata → default.
func (c *Client) opts(msgs []message.Message, tools []provider.ToolSchema, sysPrompt string) provider.CompletionOptions {
	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		if limit := c.providerOutputLimit(context.TODO()); limit > 0 {
			maxTokens = limit
		} else {
			maxTokens = defaultMaxTokens
		}
	}
	return provider.CompletionOptions{
		Model:         c.Model,
		Messages:      msgs,
		MaxTokens:     maxTokens,
		Tools:         tools,
		SystemPrompt:  sysPrompt,
		ThinkingLevel: c.ThinkingLevel,
	}
}

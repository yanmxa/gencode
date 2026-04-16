package llm

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/core"
)

// defaultMaxTokens is the fallback max output tokens when neither the caller
// nor the provider specifies a limit.
const defaultMaxTokens = 8192

// TokenUsage tracks token consumption for a conversation.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// LLM adapts a LLMProvider to core.LLM.
//
// It also provides streaming and completion methods for the loop/app layer,
// plus cumulative token usage tracking.
//
// SetThinking can be called while the agent is running.
// Changes take effect on the next Infer/Stream call.
type LLM struct {
	mu            sync.RWMutex
	provider      LLMProvider
	model         string
	maxTokens     int
	thinkingLevel ThinkingLevel
	tokens        TokenUsage
}

// NewLLM wraps an existing provider as a core.LLM with streaming and
// completion support. maxTokens=0 means resolve from provider metadata
// or fall back to defaultMaxTokens.
func NewLLM(p LLMProvider, model string, maxTokens int) *LLM {
	return &LLM{provider: p, model: model, maxTokens: maxTokens}
}

// ---------------------------------------------------------------------------
// core.LLM interface
// ---------------------------------------------------------------------------

func (l *LLM) Infer(ctx context.Context, req core.InferRequest) (<-chan core.Chunk, error) {
	l.mu.RLock()
	p := l.provider
	model := l.model
	maxTokens := l.maxTokens
	thinking := l.thinkingLevel
	l.mu.RUnlock()

	opts := CompletionOptions{
		Model:         model,
		Messages:      toProviderMessages(req.Messages),
		Tools:         req.Tools,
		SystemPrompt:  req.System,
		MaxTokens:     maxTokens,
		ThinkingLevel: thinking,
	}

	srcCh := p.Stream(ctx, opts)

	ch := make(chan core.Chunk, 8)
	go func() {
		defer close(ch)
		for sc := range srcCh {
			switch sc.Type {
			case core.ChunkTypeText:
				ch <- core.Chunk{Text: sc.Text}
			case core.ChunkTypeThinking:
				ch <- core.Chunk{Thinking: sc.Text}
			case core.ChunkTypeDone:
				ch <- core.Chunk{Done: true, Response: toInferResponse(sc.Response)}
			case core.ChunkTypeError:
				ch <- core.Chunk{Err: sc.Error}
				return
			}
		}
	}()

	return ch, nil
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// SetThinking changes the thinking/reasoning level.
func (l *LLM) SetThinking(level ThinkingLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.thinkingLevel = level
}

// ThinkingLevel returns the current thinking/reasoning level.
func (l *LLM) ThinkingLevel() ThinkingLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.thinkingLevel
}

// ---------------------------------------------------------------------------
// Streaming & Completion (used by loop/app layer)
// ---------------------------------------------------------------------------

// Stream starts a streaming completion request and returns a chunk channel.
func (l *LLM) Stream(ctx context.Context, msgs []core.Message,
	tools []ToolSchema, sysPrompt string,
) <-chan core.StreamChunk {
	return l.provider.Stream(ctx, l.completionOpts(msgs, tools, sysPrompt))
}

// Complete sends a one-shot completion (custom max tokens, no tools).
// Used for utility calls like conversation compaction.
func (l *LLM) Complete(ctx context.Context,
	sysPrompt string, msgs []core.Message, maxTokens int,
) (core.CompletionResponse, error) {
	l.mu.RLock()
	model := l.model
	p := l.provider
	l.mu.RUnlock()

	return Complete(ctx, p, CompletionOptions{
		Model:        model,
		SystemPrompt: sysPrompt,
		Messages:     msgs,
		MaxTokens:    maxTokens,
	})
}

// send sends a non-streaming completion request and returns the full response.
func (l *LLM) send(ctx context.Context, msgs []core.Message,
	tools []ToolSchema, sysPrompt string,
) (core.CompletionResponse, error) {
	return Complete(ctx, l.provider, l.completionOpts(msgs, tools, sysPrompt))
}

// ---------------------------------------------------------------------------
// Token Tracking
// ---------------------------------------------------------------------------

// AddUsage accumulates token usage from a completion response.
func (l *LLM) AddUsage(usage core.Usage) {
	l.mu.Lock()
	l.tokens.InputTokens += usage.InputTokens
	l.tokens.OutputTokens += usage.OutputTokens
	l.tokens.TotalTokens = l.tokens.InputTokens + l.tokens.OutputTokens
	l.mu.Unlock()
}

// Tokens returns the accumulated token usage.
func (l *LLM) Tokens() TokenUsage {
	l.mu.RLock()
	t := l.tokens
	l.mu.RUnlock()
	return t
}

// ---------------------------------------------------------------------------
// Identity & Limits
// ---------------------------------------------------------------------------

// Name returns the provider name (e.g., "anthropic").
func (l *LLM) Name() string {
	l.mu.RLock()
	p := l.provider
	l.mu.RUnlock()
	if p == nil {
		return ""
	}
	return p.Name()
}

// ModelID returns the model identifier.
func (l *LLM) ModelID() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.model
}

// ResolveMaxTokens returns the effective output token limit.
// Priority: 1. Custom override (maxTokens field)
//
//	2. Provider's model metadata (OutputTokenLimit from ListModels)
//	3. Default (8192)
func (l *LLM) ResolveMaxTokens(ctx context.Context) int {
	l.mu.RLock()
	p := l.provider
	model := l.model
	mt := l.maxTokens
	l.mu.RUnlock()

	if mt > 0 {
		return mt
	}
	if limit := outputLimitFromProvider(p, model); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// completionOpts builds CompletionOptions from the LLM's current configuration.
func (l *LLM) completionOpts(msgs []core.Message, tools []ToolSchema, sysPrompt string) CompletionOptions {
	l.mu.RLock()
	model := l.model
	maxTokens := l.maxTokens
	thinking := l.thinkingLevel
	p := l.provider
	l.mu.RUnlock()

	if maxTokens <= 0 {
		if limit := outputLimitFromProvider(p, model); limit > 0 {
			maxTokens = limit
		} else {
			maxTokens = defaultMaxTokens
		}
	}
	return CompletionOptions{
		Model:         model,
		Messages:      msgs,
		MaxTokens:     maxTokens,
		Tools:         tools,
		SystemPrompt:  sysPrompt,
		ThinkingLevel: thinking,
	}
}

// outputLimitFromProvider queries the provider for the model's output token limit.
func outputLimitFromProvider(p LLMProvider, model string) int {
	if p == nil {
		return 0
	}
	models, err := p.ListModels(context.TODO())
	if err != nil {
		return 0
	}
	for _, m := range models {
		if m.ID == model {
			return m.OutputTokenLimit
		}
	}
	return 0
}

// toProviderMessages converts core messages for provider consumption.
// Key semantic change: RoleTool messages become RoleUser with ToolResult.
func toProviderMessages(msgs []core.Message) []core.Message {
	out := make([]core.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case core.RoleUser:
			out = append(out, core.Message{
				Role:    core.RoleUser,
				Content: m.Content,
				Images:  m.Images,
			})
		case core.RoleAssistant:
			out = append(out, core.Message{
				Role:              core.RoleAssistant,
				Content:           m.Content,
				Thinking:          m.Thinking,
				ThinkingSignature: m.ThinkingSignature,
				ToolCalls:         m.ToolCalls,
			})
		case core.RoleTool:
			if m.ToolResult != nil {
				out = append(out, core.Message{
					Role:       core.RoleUser,
					ToolResult: m.ToolResult,
				})
			}
		}
	}
	return out
}

// toInferResponse converts a CompletionResponse to an InferResponse.
func toInferResponse(r *core.CompletionResponse) *core.InferResponse {
	if r == nil {
		return nil
	}
	return &core.InferResponse{
		Content:           r.Content,
		Thinking:          r.Thinking,
		ThinkingSignature: r.ThinkingSignature,
		ToolCalls:         r.ToolCalls,
		StopReason:        core.StopReason(r.StopReason),
		TokensIn:          r.Usage.InputTokens,
		TokensOut:         r.Usage.OutputTokens,
	}
}

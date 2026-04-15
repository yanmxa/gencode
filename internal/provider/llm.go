package provider

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
)

// LLM adapts a LLMProvider to core.LLM.
//
// All fields are hot-swappable: SetProvider, SetModel, SetThinking, SetMaxTokens
// can be called while the agent is running. Changes take effect on the next Infer call.
type LLM struct {
	mu            sync.RWMutex
	provider      LLMProvider
	model         string
	maxTokens     int
	thinkingLevel ThinkingLevel
}

// NewLLM wraps an existing provider as a core.LLM.
func NewLLM(p LLMProvider, model string, maxTokens int) *LLM {
	return &LLM{provider: p, model: model, maxTokens: maxTokens}
}

// SetProvider replaces the underlying provider (e.g. switching from Anthropic to OpenAI).
func (l *LLM) SetProvider(p LLMProvider) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.provider = p
}

// SetModel changes the model ID (e.g. switching from sonnet to opus).
func (l *LLM) SetModel(model string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.model = model
}

// SetThinking changes the thinking/reasoning level.
func (l *LLM) SetThinking(level ThinkingLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.thinkingLevel = level
}

// SetMaxTokens changes the output token limit.
func (l *LLM) SetMaxTokens(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxTokens = n
}

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
		Tools:         toProviderTools(req.Tools),
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
			case message.ChunkTypeText:
				ch <- core.Chunk{Text: sc.Text}
			case message.ChunkTypeThinking:
				ch <- core.Chunk{Thinking: sc.Text}
			case message.ChunkTypeDone:
				ch <- core.Chunk{Done: true, Response: toInferResponse(sc.Response)}
			case message.ChunkTypeError:
				ch <- core.Chunk{Err: sc.Error}
				return
			}
		}
	}()

	return ch, nil
}

// --- core.Message → message.Message ---

func toProviderMessages(msgs []core.Message) []message.Message {
	out := make([]message.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case core.RoleUser:
			out = append(out, message.Message{
				Role:    message.RoleUser,
				Content: m.Content,
				Images:  toProviderImages(m.Images),
			})
		case core.RoleAssistant:
			out = append(out, message.Message{
				Role:      message.RoleAssistant,
				Content:   m.Content,
				Thinking:  m.Thinking,
				ToolCalls: toProviderToolCalls(m.ToolCalls),
			})
		case core.RoleTool:
			if m.ToolResult != nil {
				out = append(out, message.Message{
					Role: message.RoleUser,
					ToolResult: &message.ToolResult{
						ToolCallID: m.ToolResult.ToolCallID,
						ToolName:   m.ToolResult.ToolName,
						Content:    m.ToolResult.Content,
						IsError:    m.ToolResult.IsError,
					},
				})
			}
		}
	}
	return out
}

func toProviderImages(imgs []core.Image) []message.ImageData {
	if len(imgs) == 0 {
		return nil
	}
	out := make([]message.ImageData, len(imgs))
	for i, img := range imgs {
		out[i] = message.ImageData{
			MediaType: img.MediaType,
			Data:      img.Data,
			FileName:  img.FileName,
			Size:      img.Size,
		}
	}
	return out
}

func toProviderToolCalls(calls []core.ToolCall) []message.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]message.ToolCall, len(calls))
	for i, tc := range calls {
		inputJSON, _ := json.Marshal(tc.Input)
		out[i] = message.ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: string(inputJSON),
		}
	}
	return out
}

func toProviderTools(schemas []core.ToolSchema) []message.ToolSchema {
	if len(schemas) == 0 {
		return nil
	}
	out := make([]message.ToolSchema, len(schemas))
	for i, s := range schemas {
		out[i] = message.ToolSchema{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.Parameters,
		}
	}
	return out
}

// --- message.CompletionResponse → core.InferResponse ---

func toInferResponse(r *message.CompletionResponse) *core.InferResponse {
	if r == nil {
		return nil
	}
	return &core.InferResponse{
		Content:    r.Content,
		Thinking:   r.Thinking,
		ToolCalls:  toCoreToolCalls(r.ToolCalls),
		StopReason: core.StopReason(r.StopReason),
		TokensIn:   r.Usage.InputTokens,
		TokensOut:  r.Usage.OutputTokens,
	}
}

func toCoreToolCalls(calls []message.ToolCall) []core.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]core.ToolCall, len(calls))
	for i, tc := range calls {
		input := make(map[string]any)
		_ = json.Unmarshal([]byte(tc.Input), &input)
		out[i] = core.ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		}
	}
	return out
}

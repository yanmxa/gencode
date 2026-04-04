package streamutil

import (
	"context"
	"sort"
	"time"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
)

// State tracks common streaming response state across provider implementations.
type State struct {
	ProviderName string
	Start        time.Time
	ChunkCount   int
	Response     message.CompletionResponse
}

// NewState creates a new stream state for a provider.
func NewState(providerName string) *State {
	return &State{
		ProviderName: providerName,
		Start:        time.Now(),
	}
}

// Count records one more upstream stream event/chunk.
func (s *State) Count() {
	s.ChunkCount++
}

// EmitText forwards a text delta and accumulates it into the response.
func (s *State) EmitText(ch chan<- message.StreamChunk, text string) {
	if text == "" {
		return
	}
	ch <- message.StreamChunk{
		Type: message.ChunkTypeText,
		Text: text,
	}
	s.Response.Content += text
}

// EmitThinking forwards a thinking delta and accumulates it into the response.
func (s *State) EmitThinking(ch chan<- message.StreamChunk, text string) {
	if text == "" {
		return
	}
	ch <- message.StreamChunk{
		Type: message.ChunkTypeThinking,
		Text: text,
	}
	s.Response.Thinking += text
}

// EmitToolStart forwards a tool start event.
func (s *State) EmitToolStart(ch chan<- message.StreamChunk, toolID, toolName string) {
	ch <- message.StreamChunk{
		Type:     message.ChunkTypeToolStart,
		ToolID:   toolID,
		ToolName: toolName,
	}
}

// EmitToolInput forwards a tool input delta.
func (s *State) EmitToolInput(ch chan<- message.StreamChunk, toolID, text string) {
	if text == "" {
		return
	}
	ch <- message.StreamChunk{
		Type:   message.ChunkTypeToolInput,
		ToolID: toolID,
		Text:   text,
	}
}

// UpdateUsage updates the tracked usage values when the provider emits them.
func (s *State) UpdateUsage(inputTokens, outputTokens int) {
	if inputTokens > 0 {
		s.Response.Usage.InputTokens = inputTokens
	}
	if outputTokens > 0 {
		s.Response.Usage.OutputTokens = outputTokens
	}
}

// AddToolCallsSorted appends tool calls from an indexed accumulator in stable index order.
func (s *State) AddToolCallsSorted(toolCalls map[int]*message.ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	indexes := make([]int, 0, len(toolCalls))
	for idx := range toolCalls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	for _, idx := range indexes {
		s.Response.ToolCalls = append(s.Response.ToolCalls, *toolCalls[idx])
	}
}

// AddToolCallsByKey appends tool calls from a string-keyed accumulator in stable key order.
func (s *State) AddToolCallsByKey(toolCalls map[string]*message.ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	keys := make([]string, 0, len(toolCalls))
	for key := range toolCalls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.Response.ToolCalls = append(s.Response.ToolCalls, *toolCalls[key])
	}
}

// EnsureToolUseStopReason infers tool_use when tool calls exist but no stop reason was set.
func (s *State) EnsureToolUseStopReason() {
	if len(s.Response.ToolCalls) > 0 && s.Response.StopReason == "" {
		s.Response.StopReason = "tool_use"
	}
}

// Fail logs and emits a terminal error chunk.
func (s *State) Fail(ch chan<- message.StreamChunk, err error) {
	log.LogError(s.ProviderName, err)
	ch <- message.StreamChunk{
		Type:  message.ChunkTypeError,
		Error: err,
	}
}

// Finish logs stream completion, logs the final response, and emits the done chunk.
func (s *State) Finish(ctx context.Context, ch chan<- message.StreamChunk) {
	log.LogStreamDone(s.ProviderName, time.Since(s.Start), s.ChunkCount)
	log.LogResponseCtx(ctx, s.ProviderName, s.Response)
	ch <- message.StreamChunk{
		Type:     message.ChunkTypeDone,
		Response: &s.Response,
	}
}

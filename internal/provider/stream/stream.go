package stream

import (
	"context"
	"maps"
	"slices"
	"strings"
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

	contentBuf  strings.Builder
	thinkingBuf strings.Builder
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
	s.contentBuf.WriteString(text)
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
	s.thinkingBuf.WriteString(text)
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

// UpdateCacheUsage records prompt-caching token counts from the provider response.
func (s *State) UpdateCacheUsage(cacheCreation, cacheRead int) {
	if cacheCreation > 0 {
		s.Response.Usage.CacheCreationInputTokens = cacheCreation
	}
	if cacheRead > 0 {
		s.Response.Usage.CacheReadInputTokens = cacheRead
	}
}

// AddToolCallsSorted appends tool calls from an indexed accumulator in stable index order.
func (s *State) AddToolCallsSorted(toolCalls map[int]*message.ToolCall) {
	for _, idx := range slices.Sorted(maps.Keys(toolCalls)) {
		s.Response.ToolCalls = append(s.Response.ToolCalls, *toolCalls[idx])
	}
}

// AddToolCallsByKey appends tool calls from a string-keyed accumulator in stable key order.
func (s *State) AddToolCallsByKey(toolCalls map[string]*message.ToolCall) {
	for _, key := range slices.Sorted(maps.Keys(toolCalls)) {
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
// It copies the response so the receiver does not retain a pointer into State,
// allowing the State (and its string builders) to be GC'd.
func (s *State) Finish(ctx context.Context, ch chan<- message.StreamChunk) {
	s.Response.Content = s.contentBuf.String()
	s.Response.Thinking = s.thinkingBuf.String()
	log.LogStreamDone(s.ProviderName, time.Since(s.Start), s.ChunkCount)
	log.LogResponseCtx(ctx, s.ProviderName, s.Response)
	resp := s.Response // shallow copy — breaks the pointer into State
	ch <- message.StreamChunk{
		Type:     message.ChunkTypeDone,
		Response: &resp,
	}
}

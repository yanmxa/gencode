package client

import (
	"context"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// FakeClient is a test double that returns predefined responses.
// It supports both streaming and non-streaming modes, tool calls,
// and multiple sequential responses for multi-turn conversations.
//
// Usage:
//
//	fake := &client.FakeClient{
//	    Responses: []message.CompletionResponse{
//	        {Content: "hello", StopReason: "end_turn"},
//	    },
//	}
//	// Use fake.Send() or fake.Stream() — both draw from the same Responses slice.
type FakeClient struct {
	// Responses is the queue of responses to return, consumed in order.
	// Each call to Send/Stream pops the first entry. If exhausted,
	// a default "no more responses" reply is returned.
	Responses []message.CompletionResponse

	// Model name (defaults to "fake-model")
	Model string

	// ProviderName (defaults to "fake")
	ProviderName string

	// Calls records every set of CompletionOptions received, in order.
	Calls []provider.CompletionOptions

	// ErrorAt injects an error on the Nth call (1-based). 0 means disabled.
	ErrorAt int

	// ErrorValue is the error to inject when ErrorAt triggers.
	ErrorValue error

	// callCount tracks total calls across Send/Stream/Complete.
	callCount int
}

// Send returns the next response synchronously.
func (f *FakeClient) Send(_ context.Context, msgs []message.Message,
	tools []provider.Tool, sysPrompt string) (message.CompletionResponse, error) {
	f.recordCall(msgs, tools, sysPrompt)
	if f.shouldInjectError() {
		return message.CompletionResponse{}, f.ErrorValue
	}
	return f.next(), nil
}

// Stream returns the next response as a single-chunk stream.
func (f *FakeClient) Stream(_ context.Context, msgs []message.Message,
	tools []provider.Tool, sysPrompt string) <-chan message.StreamChunk {
	f.recordCall(msgs, tools, sysPrompt)
	ch := make(chan message.StreamChunk, 1)

	var chunk message.StreamChunk
	if f.shouldInjectError() {
		chunk = message.StreamChunk{Type: message.ChunkTypeError, Error: f.ErrorValue}
	} else {
		resp := f.next()
		chunk = message.StreamChunk{Type: message.ChunkTypeDone, Response: &resp}
	}

	go func() {
		ch <- chunk
		close(ch)
	}()
	return ch
}

// Complete returns the next response (used for utility calls like compaction).
func (f *FakeClient) Complete(_ context.Context,
	sysPrompt string, msgs []message.Message, maxTokens int) (message.CompletionResponse, error) {
	f.Calls = append(f.Calls, provider.CompletionOptions{
		Model:        f.modelID(),
		SystemPrompt: sysPrompt,
		Messages:     msgs,
		MaxTokens:    maxTokens,
	})
	if f.shouldInjectError() {
		return message.CompletionResponse{}, f.ErrorValue
	}
	return f.next(), nil
}

// Name returns the provider name.
func (f *FakeClient) Name() string {
	if f.ProviderName != "" {
		return f.ProviderName
	}
	return "fake"
}

// ModelID returns the model identifier.
func (f *FakeClient) ModelID() string {
	return f.modelID()
}

// ResolveMaxTokens returns a fixed default for testing.
func (f *FakeClient) ResolveMaxTokens(_ context.Context) int {
	return defaultMaxTokens
}

// --- helpers ---

// shouldInjectError increments callCount and returns true when ErrorAt matches.
func (f *FakeClient) shouldInjectError() bool {
	f.callCount++
	return f.ErrorAt > 0 && f.callCount == f.ErrorAt
}

func (f *FakeClient) next() message.CompletionResponse {
	if len(f.Responses) == 0 {
		return message.CompletionResponse{
			Content:    "no more responses",
			StopReason: "end_turn",
		}
	}
	resp := f.Responses[0]
	f.Responses = f.Responses[1:]
	return resp
}

func (f *FakeClient) modelID() string {
	if f.Model != "" {
		return f.Model
	}
	return "fake-model"
}

func (f *FakeClient) recordCall(msgs []message.Message, tools []provider.Tool, sysPrompt string) {
	f.Calls = append(f.Calls, provider.CompletionOptions{
		Model:        f.modelID(),
		Messages:     msgs,
		Tools:        tools,
		SystemPrompt: sysPrompt,
	})
}

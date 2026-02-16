// Package testutil provides shared test helpers for integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// ---------------------------------------------------------------------------
// Loop construction helpers
// ---------------------------------------------------------------------------

// NewTestLoop creates a core.Loop with a FakeClient, PermitAll permission,
// and a temp cwd. Responses are queued in order.
func NewTestLoop(t *testing.T, responses ...message.CompletionResponse) (*core.Loop, *client.FakeClient) {
	t.Helper()
	return NewTestLoopWithPermission(t, permission.PermitAll(), responses...)
}

// NewTestLoopWithPermission creates a Loop with a custom permission checker.
func NewTestLoopWithPermission(t *testing.T, checker permission.Checker,
	responses ...message.CompletionResponse) (*core.Loop, *client.FakeClient) {
	t.Helper()

	fake := &client.FakeClient{Responses: responses}
	loop := &core.Loop{
		System:     &system.System{Cwd: t.TempDir(), Memory: "test"},
		Client:     NewTestClient(fake),
		Tool:       &tool.Set{},
		Permission: checker,
	}
	return loop, fake
}

// NewTestClient wraps a FakeClient in a client.Client ready for use in loops
// or compact calls. This avoids repeating the FakeProvider wiring in every test.
func NewTestClient(fake *client.FakeClient) *client.Client {
	return &client.Client{
		Provider:  &FakeProvider{Client: fake},
		Model:     "fake-model",
		MaxTokens: 8192,
	}
}

// ---------------------------------------------------------------------------
// Response builders
// ---------------------------------------------------------------------------

// ToolCallResponse builds a CompletionResponse that triggers a single tool_use.
func ToolCallResponse(toolName, toolID, input string) message.CompletionResponse {
	return message.CompletionResponse{
		StopReason: "tool_use",
		ToolCalls:  []message.ToolCall{{ID: toolID, Name: toolName, Input: input}},
		Usage:      message.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// MultiToolCallResponse builds a CompletionResponse with multiple tool calls.
func MultiToolCallResponse(calls ...message.ToolCall) message.CompletionResponse {
	return message.CompletionResponse{
		StopReason: "tool_use",
		ToolCalls:  calls,
		Usage:      message.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// EndTurnResponse builds a simple end_turn response with default usage.
func EndTurnResponse(content string) message.CompletionResponse {
	return message.CompletionResponse{
		Content:    content,
		StopReason: "end_turn",
		Usage:      message.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// EndTurnResponseWithUsage builds an end_turn response with custom token counts.
func EndTurnResponseWithUsage(content string, input, output int) message.CompletionResponse {
	return message.CompletionResponse{
		Content:    content,
		StopReason: "end_turn",
		Usage:      message.Usage{InputTokens: input, OutputTokens: output},
	}
}

// ---------------------------------------------------------------------------
// Fake tool registration
// ---------------------------------------------------------------------------

// RegisterFakeTool registers a named tool in the global registry that returns
// a fixed result. The global registry is reset via t.Cleanup.
func RegisterFakeTool(t *testing.T, name, result string) {
	t.Helper()
	tool.Register(&fakeTool{name: name, result: result})
	t.Cleanup(func() { tool.DefaultRegistry = tool.NewRegistry() })
}

type fakeTool struct {
	name   string
	result string
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake tool for testing" }
func (f *fakeTool) Icon() string        { return "T" }
func (f *fakeTool) Execute(_ context.Context, _ map[string]any, _ string) ui.ToolResult {
	return ui.ToolResult{
		Success:  true,
		Output:   f.result,
		Metadata: ui.ResultMetadata{Title: f.name},
	}
}

// ---------------------------------------------------------------------------
// Fake / mock providers
// ---------------------------------------------------------------------------

// FakeProvider wraps a FakeClient as a provider.LLMProvider.
// Use this when the code under test expects a provider.LLMProvider and you
// want to control responses via FakeClient.
type FakeProvider struct {
	Client *client.FakeClient
}

func (p *FakeProvider) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	return p.Client.Stream(ctx, opts.Messages, opts.Tools, opts.SystemPrompt)
}
func (p *FakeProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (p *FakeProvider) Name() string                                               { return p.Client.Name() }

// MockProvider is a standalone provider.LLMProvider backed by a response queue.
// Unlike FakeProvider, it does not require a FakeClient — use this when the
// code under test (e.g., agent.Executor) creates its own client internally.
type MockProvider struct {
	Responses []message.CompletionResponse
	callIdx   int
}

func (m *MockProvider) Stream(_ context.Context, _ provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk, 1)
	go func() {
		defer close(ch)
		var resp message.CompletionResponse
		if m.callIdx < len(m.Responses) {
			resp = m.Responses[m.callIdx]
			m.callIdx++
		} else {
			resp = message.CompletionResponse{Content: "no more responses", StopReason: "end_turn"}
		}
		ch <- message.StreamChunk{Type: message.ChunkTypeDone, Response: &resp}
	}()
	return ch
}
func (m *MockProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (m *MockProvider) Name() string                                               { return "mock" }

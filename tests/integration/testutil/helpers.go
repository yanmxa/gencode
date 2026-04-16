// Package testutil provides shared test helpers for integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/loop"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// ---------------------------------------------------------------------------
// Loop construction helpers
// ---------------------------------------------------------------------------

// NewTestLoop creates a loop.Loop with a FakeClient, PermitAll permission,
// and a temp cwd. Responses are queued in order.
func NewTestLoop(t *testing.T, responses ...core.CompletionResponse) (*loop.Loop, *provider.FakeLLM) {
	t.Helper()
	return NewTestLoopWithPermission(t, permission.PermitAll(), responses...)
}

// NewTestLoopWithPermission creates a Loop with a custom permission checker.
func NewTestLoopWithPermission(t *testing.T, checker permission.Checker,
	responses ...core.CompletionResponse,
) (*loop.Loop, *provider.FakeLLM) {
	t.Helper()

	tmpDir := t.TempDir()
	fake := &provider.FakeLLM{Responses: responses}
	lp := &loop.Loop{
		System:     core.NewSystem(core.Layer{Name: "test", Priority: 0, Content: "test"}),
		Client:     NewTestClient(fake),
		Tool:       &tool.Set{},
		Permission: checker,
		Cwd:        tmpDir,
	}
	return lp, fake
}

// NewTestClient wraps a FakeLLM in a provider.LLM ready for use in loops
// or compact calls. This avoids repeating the FakeProvider wiring in every test.
func NewTestClient(fake *provider.FakeLLM) *provider.LLM {
	return provider.NewLLM(&FakeProvider{Client: fake}, "fake-model", 8192)
}

// ---------------------------------------------------------------------------
// Response builders
// ---------------------------------------------------------------------------

// ToolCallResponse builds a CompletionResponse that triggers a single tool_use.
func ToolCallResponse(toolName, toolID, input string) core.CompletionResponse {
	return core.CompletionResponse{
		StopReason: "tool_use",
		ToolCalls:  []core.ToolCall{{ID: toolID, Name: toolName, Input: input}},
		Usage:      core.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// MultiToolCallResponse builds a CompletionResponse with multiple tool calls.
func MultiToolCallResponse(calls ...core.ToolCall) core.CompletionResponse {
	return core.CompletionResponse{
		StopReason: "tool_use",
		ToolCalls:  calls,
		Usage:      core.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// EndTurnResponse builds a simple end_turn response with default usage.
func EndTurnResponse(content string) core.CompletionResponse {
	return core.CompletionResponse{
		Content:    content,
		StopReason: "end_turn",
		Usage:      core.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

// EndTurnResponseWithUsage builds an end_turn response with custom token counts.
func EndTurnResponseWithUsage(content string, input, output int) core.CompletionResponse {
	return core.CompletionResponse{
		Content:    content,
		StopReason: "end_turn",
		Usage:      core.Usage{InputTokens: input, OutputTokens: output},
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
func (f *fakeTool) Execute(_ context.Context, _ map[string]any, _ string) toolresult.ToolResult {
	return toolresult.ToolResult{
		Success:  true,
		Output:   f.result,
		Metadata: toolresult.ResultMetadata{Title: f.name},
	}
}

// ---------------------------------------------------------------------------
// Fake / mock providers
// ---------------------------------------------------------------------------

// FakeProvider wraps a FakeClient as a provider.LLMProvider.
// Use this when the code under test expects a provider.LLMProvider and you
// want to control responses via FakeClient.
type FakeProvider struct {
	Client *provider.FakeLLM
}

func (p *FakeProvider) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan core.StreamChunk {
	return p.Client.Stream(ctx, opts.Messages, opts.Tools, opts.SystemPrompt)
}
func (p *FakeProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (p *FakeProvider) Name() string                                               { return p.Client.Name() }

// MockProvider is a standalone provider.LLMProvider backed by a response queue.
// Unlike FakeProvider, it does not require a FakeClient — use this when the
// code under test (e.g., agent.Executor) creates its own client internally.
type MockProvider struct {
	Responses []core.CompletionResponse
	callIdx   int
}

func (m *MockProvider) Stream(_ context.Context, _ provider.CompletionOptions) <-chan core.StreamChunk {
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		var resp core.CompletionResponse
		if m.callIdx < len(m.Responses) {
			resp = m.Responses[m.callIdx]
			m.callIdx++
		} else {
			resp = core.CompletionResponse{Content: "no more responses", StopReason: "end_turn"}
		}
		ch <- core.StreamChunk{Type: core.ChunkTypeDone, Response: &resp}
	}()
	return ch
}
func (m *MockProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (m *MockProvider) Name() string                                               { return "mock" }

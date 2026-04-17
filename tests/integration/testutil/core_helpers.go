package testutil

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
)

// FakeLLM implements core.LLM for testing, returning queued responses.
type FakeLLM struct {
	Responses []core.CompletionResponse
	callIdx   int
}

func (f *FakeLLM) InputLimit() int { return 0 }

func (f *FakeLLM) Infer(_ context.Context, _ core.InferRequest) (<-chan core.Chunk, error) {
	ch := make(chan core.Chunk, 1)
	go func() {
		defer close(ch)
		var resp core.CompletionResponse
		if f.callIdx < len(f.Responses) {
			resp = f.Responses[f.callIdx]
			f.callIdx++
		} else {
			resp = core.CompletionResponse{Content: "no more responses", StopReason: "end_turn"}
		}
		// Convert via bridge's toInferResponse path
		ch <- core.Chunk{
			Done: true,
			Response: &core.InferResponse{
				Content:    resp.Content,
				Thinking:   resp.Thinking,
				ToolCalls:  legacyToCoreCalls(resp.ToolCalls),
				StopReason: core.StopReason(resp.StopReason),
				TokensIn:   resp.Usage.InputTokens,
				TokensOut:  resp.Usage.OutputTokens,
			},
		}
	}()
	return ch, nil
}

func legacyToCoreCalls(calls []core.ToolCall) []core.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]core.ToolCall, len(calls))
	copy(out, calls)
	return out
}

// NewTestAgent creates a core.Agent backed by a FakeLLM with queued responses.
// All globally registered tools (including dynamically registered fakes) are included.
func NewTestAgent(t *testing.T, responses ...core.CompletionResponse) (core.Agent, *FakeLLM) {
	t.Helper()
	fakeLLM := &FakeLLM{Responses: responses}
	cwd := t.TempDir()
	return core.NewAgent(core.Config{
		ID:       "test-agent",
		LLM:      fakeLLM,
		System:   core.NewSystem(),
		Tools:    buildAllRegisteredTools(cwd),
		Hooks:    core.NewHooks(),
		CWD:      cwd,
		MaxTurns: 100,
	}), fakeLLM
}

// buildAllRegisteredTools creates a core.Tools wrapping ALL tools in the global registry,
// including dynamically registered fake tools. Unlike AdaptToolRegistry which only finds
// tools that have schemas in GetToolSchemas(), this walks the entire registry directly.
func buildAllRegisteredTools(cwd string) core.Tools {
	var adapted []core.Tool
	for _, name := range tool.DefaultRegistry.List() {
		t, ok := tool.Get(name)
		if !ok {
			continue
		}
		schema := core.ToolSchema{Name: name, Description: t.Description()}
		adapted = append(adapted, tool.AdaptTool(t, schema, func() string { return cwd }))
	}
	return core.NewTools(adapted...)
}

// BuildTestTools adapts all globally registered tools into a core.Tools for use in tests.
func BuildTestTools(t *testing.T) core.Tools {
	t.Helper()
	return buildAllRegisteredTools(t.TempDir())
}

// RunAgent sends a prompt to the agent, drains its outbox, and returns the result.
// It sends SigStop after the first OnTurn event (single cycle).
func RunAgent(ctx context.Context, ag core.Agent, prompt string) (core.Result, error) {
	var agentErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		agentErr = ag.Run(ctx)
	}()

	ag.Inbox() <- core.Message{Role: core.RoleUser, Content: prompt}

	var result core.Result
	var hasResult bool
	for ev := range ag.Outbox() {
		if ev.Type == core.OnTurn {
			if r, ok := ev.Result(); ok {
				result = r
				hasResult = true
			}
			ag.Inbox() <- core.Message{Signal: core.SigStop}
		}
	}

	<-done

	if agentErr != nil && !hasResult {
		return result, agentErr
	}
	return result, nil
}

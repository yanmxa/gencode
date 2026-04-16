package toolui

import (
	"context"

	"github.com/yanmxa/gencode/internal/core"
)

// State holds tool selector and execution state for the TUI model.
type State struct {
	Selector Model
	ExecState
}

// ExecState holds tool execution state for the TUI model.
// In the agent path, PendingCalls is always nil — tools execute inside
// the core.Agent goroutine. These fields remain for the permission bridge
// and interrupt/resume flow in handler_input_runtime.go.
type ExecState struct {
	PendingCalls []core.ToolCall
	CurrentIdx   int
	Ctx          context.Context
	Cancel       context.CancelFunc
}

// Begin initializes a fresh execution context for a tool run and returns it.
func (t *ExecState) Begin() context.Context {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Ctx, t.Cancel = context.WithCancel(context.Background())
	return t.Ctx
}

// Context returns the active execution context, or Background when idle.
func (t *ExecState) Context() context.Context {
	if t.Ctx != nil {
		return t.Ctx
	}
	return context.Background()
}

// Reset clears all tool execution state.
func (t *ExecState) Reset() {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.PendingCalls = nil
	t.CurrentIdx = 0
	t.Ctx = nil
	t.Cancel = nil
}

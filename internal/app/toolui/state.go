package toolui

import (
	"context"

	"github.com/yanmxa/gencode/internal/message"
)

// State holds tool selector and execution state for the TUI model.
type State struct {
	Selector Model
	ExecState
}

// ExecState holds tool execution state for the TUI model.
type ExecState struct {
	PendingCalls    []message.ToolCall
	CurrentIdx      int
	Parallel        bool
	ParallelResults map[int]message.ToolResult
	ParallelCount   int
	HookAllowed     map[string]bool // Tool call IDs pre-approved by hooks
	HookForceAsk    map[string]bool // Tool call IDs forced to prompt by hooks (PreToolUse "ask")
	HookContext     string          // Deferred AdditionalContext from PreToolUse hooks
	Ctx             context.Context
	Cancel          context.CancelFunc
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
	t.Parallel = false
	t.ParallelResults = nil
	t.ParallelCount = 0
	t.HookAllowed = nil
	t.HookForceAsk = nil
	t.HookContext = ""
	t.Ctx = nil
	t.Cancel = nil
}

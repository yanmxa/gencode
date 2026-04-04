package tool

import "github.com/yanmxa/gencode/internal/message"

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
}

// Reset clears all tool execution state.
func (t *ExecState) Reset() {
	t.PendingCalls = nil
	t.CurrentIdx = 0
	t.Parallel = false
	t.ParallelResults = nil
	t.ParallelCount = 0
	t.HookAllowed = nil
}

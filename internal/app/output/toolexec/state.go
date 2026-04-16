package toolexec

import (
	"context"

	"github.com/yanmxa/gencode/internal/core"
)

// ExecState holds tool execution state for the TUI model.
type ExecState struct {
	PendingCalls []core.ToolCall
	CurrentIdx   int
	Ctx          context.Context
	Cancel       context.CancelFunc
}

func (t *ExecState) Begin() context.Context {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Ctx, t.Cancel = context.WithCancel(context.Background())
	return t.Ctx
}

func (t *ExecState) Context() context.Context {
	if t.Ctx != nil {
		return t.Ctx
	}
	return context.Background()
}

func (t *ExecState) Reset() {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.PendingCalls = nil
	t.CurrentIdx = 0
	t.Ctx = nil
	t.Cancel = nil
}

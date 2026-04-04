package tool

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/message"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type cancellableTestTool struct{}

func (t *cancellableTestTool) Name() string        { return "AppToolCancellationTestTool" }
func (t *cancellableTestTool) Description() string { return "test tool" }
func (t *cancellableTestTool) Icon() string        { return "t" }
func (t *cancellableTestTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	select {
	case <-ctx.Done():
		return ui.NewErrorResult(t.Name(), "cancelled")
	default:
		return ui.ToolResult{Success: true, Output: "not cancelled"}
	}
}

func TestExecuteParallelPropagatesContextCancellation(t *testing.T) {
	coretool.Register(&cancellableTestTool{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := ExecuteParallel(ctx, nil, []message.ToolCall{{
		ID:    "tc1",
		Name:  "AppToolCancellationTestTool",
		Input: `{}`,
	}}, "", nil, nil, false, nil)

	msg := cmd()
	result, ok := msg.(ExecResultMsg)
	if !ok {
		t.Fatalf("expected ExecResultMsg, got %T", msg)
	}
	if !result.Result.IsError {
		t.Fatal("expected cancelled execution to produce an error result")
	}
	if result.Result.Content != "Error: cancelled" {
		t.Fatalf("expected cancellation content, got %q", result.Result.Content)
	}
}

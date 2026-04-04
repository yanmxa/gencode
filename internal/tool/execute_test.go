package tool

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type testPermissionAwareTool struct{}

func (t *testPermissionAwareTool) Name() string        { return "TestPermissionAwareTool" }
func (t *testPermissionAwareTool) Description() string { return "test tool" }
func (t *testPermissionAwareTool) Icon() string        { return "t" }
func (t *testPermissionAwareTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return ui.ToolResult{Success: true, Output: "execute"}
}
func (t *testPermissionAwareTool) RequiresPermission() bool { return true }
func (t *testPermissionAwareTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	return nil, nil
}
func (t *testPermissionAwareTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return ui.ToolResult{Success: true, Output: "approved"}
}

type testMCPExecutor struct {
	handled bool
}

func (e *testMCPExecutor) IsMCPTool(name string) bool {
	return name == "mcp__test__tool"
}

func (e *testMCPExecutor) ExecuteMCP(ctx context.Context, name string, params map[string]any) (ui.ToolResult, error) {
	e.handled = true
	return ui.ToolResult{Success: true, Output: "mcp"}, nil
}

func TestExecutePreparedToolUsesExecuteApprovedWhenRequested(t *testing.T) {
	Register(&testPermissionAwareTool{})

	tc := message.ToolCall{ID: "tc1", Name: "TestPermissionAwareTool"}
	result, err := ExecutePreparedTool(context.Background(), tc, map[string]any{}, "", true, nil)
	if err != nil {
		t.Fatalf("ExecutePreparedTool returned error: %v", err)
	}
	if result.Output != "approved" {
		t.Fatalf("expected approved path, got %q", result.Output)
	}
}

func TestExecutePreparedToolUsesExecuteByDefault(t *testing.T) {
	tc := message.ToolCall{ID: "tc2", Name: "TestPermissionAwareTool"}
	result, err := ExecutePreparedTool(context.Background(), tc, map[string]any{}, "", false, nil)
	if err != nil {
		t.Fatalf("ExecutePreparedTool returned error: %v", err)
	}
	if result.Output != "execute" {
		t.Fatalf("expected execute path, got %q", result.Output)
	}
}

func TestExecutePreparedToolRoutesMCPTools(t *testing.T) {
	mcpExec := &testMCPExecutor{}
	tc := message.ToolCall{ID: "tc3", Name: "mcp__test__tool"}
	result, err := ExecutePreparedTool(context.Background(), tc, map[string]any{"x": "y"}, "", false, mcpExec)
	if err != nil {
		t.Fatalf("ExecutePreparedTool returned error: %v", err)
	}
	if !mcpExec.handled {
		t.Fatal("expected MCP executor to be used")
	}
	if result.Output != "mcp" {
		t.Fatalf("expected MCP output, got %q", result.Output)
	}
}

func TestExecutePreparedToolReturnsUnknownToolError(t *testing.T) {
	tc := message.ToolCall{ID: "tc4", Name: "DefinitelyUnknownTool"}
	_, err := ExecutePreparedTool(context.Background(), tc, map[string]any{}, "", false, nil)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
}

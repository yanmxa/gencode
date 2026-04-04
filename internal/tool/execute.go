package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// MCPExecutor executes MCP tool calls for the shared tool runtime.
type MCPExecutor interface {
	IsMCPTool(name string) bool
	ExecuteMCP(ctx context.Context, name string, params map[string]any) (ui.ToolResult, error)
}

// ExecutePreparedTool executes a parsed tool call against either the MCP executor
// or the registered built-in tool set.
//
// When approved is true, PermissionAwareTool implementations run via
// ExecuteApproved to match non-interactive callers that auto-approve after
// their own permission checks.
func ExecutePreparedTool(
	ctx context.Context,
	tc message.ToolCall,
	params map[string]any,
	cwd string,
	approved bool,
	mcpExec MCPExecutor,
) (ui.ToolResult, error) {
	if mcpExec != nil && mcpExec.IsMCPTool(tc.Name) {
		return mcpExec.ExecuteMCP(ctx, tc.Name, params)
	}

	t, ok := Get(tc.Name)
	if !ok {
		return ui.ToolResult{}, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	if approved {
		if pat, ok := t.(PermissionAwareTool); ok && pat.RequiresPermission() {
			return pat.ExecuteApproved(ctx, params, cwd), nil
		}
	}

	return t.Execute(ctx, params, cwd), nil
}

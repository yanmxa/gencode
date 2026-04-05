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
	prepared := &PreparedToolCall{
		Call:   tc,
		Params: params,
	}

	if resolved, ok := Get(tc.Name); ok {
		prepared.Tool = resolved
	} else if mcpExec != nil && mcpExec.IsMCPTool(tc.Name) {
		prepared.IsMCP = true
	} else {
		return ui.ToolResult{}, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	return prepared.Execute(ctx, cwd, approved, mcpExec)
}

package tool

import (
	"context"

	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// MCPExecutor executes MCP tool calls for the shared tool runtime.
type MCPExecutor interface {
	IsMCPTool(name string) bool
	ExecuteMCP(ctx context.Context, name string, params map[string]any) (toolresult.ToolResult, error)
}

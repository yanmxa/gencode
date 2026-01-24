package tool

import (
	"context"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// Tool represents a read-only tool that can be executed
type Tool interface {
	// Name returns the tool name
	Name() string

	// Description returns a brief description of the tool
	Description() string

	// Icon returns the tool icon emoji
	Icon() string

	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult
}

// PermissionAwareTool is a tool that requires user permission before execution
type PermissionAwareTool interface {
	Tool

	// RequiresPermission returns true if the tool needs user approval
	RequiresPermission() bool

	// PreparePermission prepares a permission request (e.g., computes diff)
	PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error)

	// ExecuteApproved executes the tool after user approval
	ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult
}

// InteractiveTool is a tool that requires user interaction (not just permission)
// Examples: AskUserQuestion for collecting user input
type InteractiveTool interface {
	Tool

	// RequiresInteraction returns true if the tool needs user interaction
	RequiresInteraction() bool

	// PrepareInteraction prepares an interaction request (e.g., question prompt)
	PrepareInteraction(ctx context.Context, params map[string]any, cwd string) (interface{}, error)

	// ExecuteWithResponse executes the tool with the user's response
	ExecuteWithResponse(ctx context.Context, params map[string]any, response interface{}, cwd string) ui.ToolResult
}

// ToolInput represents parsed tool input
type ToolInput struct {
	Name   string         // Tool name
	Args   string         // Raw argument string
	Params map[string]any // Parsed parameters
}

package tool

import (
	"context"

	coreperm "github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
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
	Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult
}

// PermissionAwareTool is a tool that requires user permission before execution
type PermissionAwareTool interface {
	Tool

	// RequiresPermission returns true if the tool needs user approval
	RequiresPermission() bool

	// PreparePermission prepares a permission request (e.g., computes diff)
	PreparePermission(ctx context.Context, params map[string]any, cwd string) (*perm.PermissionRequest, error)

	// ExecuteApproved executes the tool after user approval
	ExecuteApproved(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult
}

// ToolPermissionChecker is an optional interface that tools can implement
// to provide custom permission logic. This is called early in the pipeline
// (after deny rules, before mode checks) and allows tools to make fine-grained
// decisions about specific inputs.
//
// Inspired by Claude Code's tool.checkPermissions() which returns
// allow/deny/ask/passthrough/safetyCheck.
//
// Tools that don't implement this interface are treated as returning Passthrough.
type ToolPermissionChecker interface {
	// CheckPermissions performs tool-specific permission analysis.
	// Returns a PermissionCheckResult indicating the tool's decision.
	// Passthrough means the tool has no opinion — defer to the main pipeline.
	CheckPermissions(params map[string]any, cwd string) PermissionCheckResult
}

// PermissionCheckResult carries a tool's custom permission decision.
type PermissionCheckResult struct {
	// Decision is the tool's permission verdict.
	// Use permission.Permit, Reject, Prompt, or Defer.
	Decision coreperm.Decision
	// Reason explains the decision (for logging/display).
	Reason string
	// BypassImmune means this decision cannot be overridden by BypassPermissions mode.
	BypassImmune bool
}

// InteractiveTool is a tool that requires user interaction (not just permission)
// Examples: AskUserQuestion for collecting user input
type InteractiveTool interface {
	Tool

	// RequiresInteraction returns true if the tool needs user interaction
	RequiresInteraction() bool

	// PrepareInteraction prepares an interaction request (e.g., question prompt)
	PrepareInteraction(ctx context.Context, params map[string]any, cwd string) (any, error)

	// ExecuteWithResponse executes the tool with the user's response
	ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) toolresult.ToolResult
}

// ToolInput represents parsed tool input
type ToolInput struct {
	Name   string         // Tool name
	Args   string         // Raw argument string
	Params map[string]any // Parsed parameters
}

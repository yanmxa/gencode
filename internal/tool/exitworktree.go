package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// ExitWorktreeRequest is sent to the TUI for user confirmation.
type ExitWorktreeRequest struct {
	Action         string // "keep" or "remove"
	DiscardChanges bool   // when action=remove and changes exist
}

// ExitWorktreeResponse is the TUI's response.
type ExitWorktreeResponse struct {
	Approved     bool
	RestoredPath string // original cwd restored by TUI
}

// ExitWorktreeTool exits the current worktree session, optionally removing it.
type ExitWorktreeTool struct{}

func (t *ExitWorktreeTool) Name() string        { return "ExitWorktree" }
func (t *ExitWorktreeTool) Description() string { return "Exit the current worktree and return to the original directory" }
func (t *ExitWorktreeTool) Icon() string        { return "T" }

// RequiresInteraction returns true — needs user confirmation.
func (t *ExitWorktreeTool) RequiresInteraction() bool { return true }

// PrepareInteraction returns the request for the TUI.
func (t *ExitWorktreeTool) PrepareInteraction(_ context.Context, params map[string]any, _ string) (any, error) {
	action := getString(params, "action")
	if action == "" {
		action = "remove"
	}
	if action != "keep" && action != "remove" {
		return nil, fmt.Errorf("action must be 'keep' or 'remove', got %q", action)
	}

	discardChanges := getBool(params, "discard_changes")

	return &ExitWorktreeRequest{
		Action:         action,
		DiscardChanges: discardChanges,
	}, nil
}

// ExecuteWithResponse handles the TUI's approval.
func (t *ExitWorktreeTool) ExecuteWithResponse(_ context.Context, _ map[string]any, response any, _ string) ui.ToolResult {
	resp, ok := response.(*ExitWorktreeResponse)
	if !ok || !resp.Approved {
		return ui.ToolResult{
			Success: true,
			Output:  "User declined exiting worktree. Still in worktree session.",
			Metadata: ui.ResultMetadata{Title: t.Name(), Icon: t.Icon()},
		}
	}

	return ui.ToolResult{
		Success: true,
		Output: fmt.Sprintf("Exited worktree. Restored working directory to %s.",
			resp.RestoredPath),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: "restored: " + resp.RestoredPath,
		},
	}
}

// Execute is the non-interactive fallback.
func (t *ExitWorktreeTool) Execute(_ context.Context, _ map[string]any, _ string) ui.ToolResult {
	return ui.NewErrorResult(t.Name(), "ExitWorktree requires an active worktree session (interactive mode)")
}

func init() {
	Register(&ExitWorktreeTool{})
}

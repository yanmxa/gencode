package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
	"github.com/yanmxa/gencode/internal/worktree"
)

// EnterWorktreeRequest is sent to the TUI for user confirmation.
type EnterWorktreeRequest struct {
	Slug string // optional slug for the worktree directory
}

// EnterWorktreeResponse is the TUI's response.
type EnterWorktreeResponse struct {
	Approved      bool
	WorktreePath  string // set by TUI after creating worktree
	WorktreeClean func() // cleanup function
}

// EnterWorktreeTool switches the main conversation into a git worktree
// for safe experimentation. The user must approve.
type EnterWorktreeTool struct{}

func (t *EnterWorktreeTool) Name() string        { return "EnterWorktree" }
func (t *EnterWorktreeTool) Description() string { return "Switch into a git worktree for safe experimentation" }
func (t *EnterWorktreeTool) Icon() string        { return "T" }

// RequiresInteraction returns true — needs user confirmation.
func (t *EnterWorktreeTool) RequiresInteraction() bool { return true }

// PrepareInteraction returns the request for the TUI.
func (t *EnterWorktreeTool) PrepareInteraction(_ context.Context, params map[string]any, cwd string) (any, error) {
	slug, _ := params["name"].(string)
	return &EnterWorktreeRequest{Slug: slug}, nil
}

// ExecuteWithResponse handles the TUI's approval.
func (t *EnterWorktreeTool) ExecuteWithResponse(_ context.Context, params map[string]any, response any, cwd string) ui.ToolResult {
	resp, ok := response.(*EnterWorktreeResponse)
	if !ok || !resp.Approved {
		return ui.ToolResult{
			Success: true,
			Output:  "User declined entering worktree. Continue working in the current directory.",
			Metadata: ui.ResultMetadata{Title: t.Name(), Icon: t.Icon()},
		}
	}

	return ui.ToolResult{
		Success: true,
		Output: fmt.Sprintf("Switched to worktree at %s.\n"+
			"All file operations now target this isolated copy.\n"+
			"Use ExitWorktree when done to return to the original directory.",
			resp.WorktreePath),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: resp.WorktreePath,
		},
	}
}

// Execute is the non-interactive fallback (auto-approve for subagents).
func (t *EnterWorktreeTool) Execute(_ context.Context, params map[string]any, cwd string) ui.ToolResult {
	slug, _ := params["name"].(string)

	result, cleanup, err := worktree.Create(cwd, slug)
	if err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}
	_ = cleanup // worktree lifecycle managed by agent executor

	return ui.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Created worktree at %s", result.Path),
		Metadata: ui.ResultMetadata{Title: t.Name(), Icon: t.Icon(), Subtitle: result.Path},
	}
}

func init() {
	Register(&EnterWorktreeTool{})
}

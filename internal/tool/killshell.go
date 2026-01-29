package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconKillShell = "x"
)

// KillShellTool terminates a background task
type KillShellTool struct{}

func (t *KillShellTool) Name() string        { return "KillShell" }
func (t *KillShellTool) Description() string { return "Terminate a background task" }
func (t *KillShellTool) Icon() string        { return IconKillShell }

// Execute terminates a background task
func (t *KillShellTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	shellID, ok := params["shell_id"].(string)
	if !ok || shellID == "" {
		return ui.ToolResult{
			Success: false,
			Error:   "shell_id is required",
			Metadata: ui.ResultMetadata{
				Title: t.Name(),
				Icon:  t.Icon(),
			},
		}
	}

	// Get task to check status before killing
	bgTask, found := task.DefaultManager.Get(shellID)
	if !found {
		return ui.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("task not found: %s", shellID),
			Metadata: ui.ResultMetadata{
				Title: t.Name(),
				Icon:  t.Icon(),
			},
		}
	}

	// Check if already completed
	if !bgTask.IsRunning() {
		info := bgTask.GetStatus()
		return ui.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("task already completed with status: %s", info.Status),
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("Already: %s", info.Status),
			},
		}
	}

	// Get task info before killing
	info := bgTask.GetStatus()

	// Kill the task
	err := task.DefaultManager.Kill(shellID)
	duration := time.Since(start)

	if err != nil {
		return ui.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to kill task: %v", err),
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Duration: duration,
			},
		}
	}

	// Get final status
	finalInfo := bgTask.GetStatus()

	output := fmt.Sprintf("Task killed successfully.\nTask ID: %s\nPID: %d\nStatus: %s", shellID, info.PID, finalInfo.Status)
	if finalInfo.Output != "" {
		output += fmt.Sprintf("\n\nOutput before kill:\n%s", finalInfo.Output)
	}

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("Killed: %s", shellID),
			Duration: duration,
		},
	}
}

func init() {
	Register(&KillShellTool{})
}

package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconTaskStop = "x"
)

// TaskStopTool stops a running background task
type TaskStopTool struct{}

func (t *TaskStopTool) Name() string        { return "TaskStop" }
func (t *TaskStopTool) Description() string { return "Stops a running background task by its ID" }
func (t *TaskStopTool) Icon() string        { return IconTaskStop }

// Execute stops a running background task
func (t *TaskStopTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	taskID, ok := params["task_id"].(string)
	if !ok || taskID == "" {
		return ui.ToolResult{
			Success: false,
			Error:   "task_id is required",
			Metadata: ui.ResultMetadata{
				Title: t.Name(),
				Icon:  t.Icon(),
			},
		}
	}

	// Get task to check status before stopping
	bgTask, found := task.DefaultManager.Get(taskID)
	if !found {
		return ui.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("task not found: %s", taskID),
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

	// Get task info before stopping
	info := bgTask.GetStatus()

	// Stop the task
	err := task.DefaultManager.Kill(taskID)
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

	output := fmt.Sprintf("Task stopped successfully.\nTask ID: %s\nPID: %d\nStatus: %s", taskID, info.PID, finalInfo.Status)
	if finalInfo.Output != "" {
		output += fmt.Sprintf("\n\nOutput before stop:\n%s", finalInfo.Output)
	}

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("Stopped: %s", taskID),
			Duration: duration,
		},
	}
}

func init() {
	Register(&TaskStopTool{})
}

package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconTaskOutput = ">"
)

// TaskOutputTool retrieves output from background tasks
type TaskOutputTool struct{}

func (t *TaskOutputTool) Name() string        { return "TaskOutput" }
func (t *TaskOutputTool) Description() string { return "Retrieve output from a background task" }
func (t *TaskOutputTool) Icon() string        { return IconTaskOutput }

// Execute retrieves task output
func (t *TaskOutputTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
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

	// Get block parameter (default true)
	block := true
	if b, ok := params["block"].(bool); ok {
		block = b
	}

	// Get timeout (default 30 seconds, max 600 seconds)
	timeout := 30 * time.Second
	if timeoutMs, ok := params["timeout"].(float64); ok && timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
		if timeout > 600*time.Second {
			timeout = 600 * time.Second
		}
	}

	// Get task
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

	// If blocking, wait for completion
	if block && bgTask.IsRunning() {
		completed := bgTask.WaitForCompletion(timeout)
		if !completed {
			// Timeout - return current output with timeout error
			info := bgTask.GetStatus()
			duration := time.Since(start)
			return ui.ToolResult{
				Success: false,
				Output:  info.Output,
				Error:   fmt.Sprintf("timeout waiting for task (task still running, PID: %d)", info.PID),
				Metadata: ui.ResultMetadata{
					Title:    t.Name(),
					Icon:     t.Icon(),
					Subtitle: fmt.Sprintf("Timeout: %s", taskID),
					Duration: duration,
				},
			}
		}
	}

	// Get task status
	info := bgTask.GetStatus()
	duration := time.Since(start)

	// Build output
	var statusStr string
	switch info.Status {
	case task.StatusRunning:
		statusStr = "running"
	case task.StatusCompleted:
		statusStr = "completed"
	case task.StatusFailed:
		statusStr = fmt.Sprintf("failed (exit code: %d)", info.ExitCode)
	case task.StatusKilled:
		statusStr = "killed"
	}

	output := fmt.Sprintf("Task ID: %s\nStatus: %s\nPID: %d\n", info.ID, statusStr, info.PID)
	if info.Command != "" {
		output += fmt.Sprintf("Command: %s\n", info.Command)
	}
	if !info.EndTime.IsZero() {
		output += fmt.Sprintf("Duration: %v\n", info.EndTime.Sub(info.StartTime))
	}
	if info.Output != "" {
		output += fmt.Sprintf("\nOutput:\n%s", info.Output)
	}
	if info.Error != "" {
		output += fmt.Sprintf("\nError: %s", info.Error)
	}

	return ui.ToolResult{
		Success: info.Status != task.StatusFailed,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%s: %s", taskID, statusStr),
			Duration: duration,
		},
	}
}

func init() {
	Register(&TaskOutputTool{})
}

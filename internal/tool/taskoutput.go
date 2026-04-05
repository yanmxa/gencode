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

// TaskOutputTool retrieves output from background tasks.
// Aliases: AgentOutput (deprecated).
type TaskOutputTool struct{}

func (t *TaskOutputTool) Name() string        { return "TaskOutput" }
func (t *TaskOutputTool) Description() string { return "Retrieve output from a background task" }
func (t *TaskOutputTool) Icon() string        { return IconTaskOutput }

// Execute retrieves task output
func (t *TaskOutputTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	taskID := getString(params, "task_id")
	if taskID == "" {
		return ui.ToolResult{
			Success: false,
			Error:   "task_id is required",
			Metadata: ui.ResultMetadata{
				Title: t.Name(),
				Icon:  t.Icon(),
			},
		}
	}

	// Get block parameter (default true — absent means block)
	block := true
	if b, ok := params["block"].(bool); ok {
		block = b
	}

	timeout := min(time.Duration(getFloat64(params, "timeout", 30000))*time.Millisecond, 600*time.Second)

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
		if !bgTask.WaitForCompletion(timeout) {
			// Task still running - return friendly status with options
			info := bgTask.GetStatus()
			output := formatTaskOutput(info, "still running")
			output += fmt.Sprintf("\nOptions:\n"+
				"  - Wait longer: TaskOutput(task_id=\"%s\", timeout=60000)\n"+
				"  - Check status: TaskOutput(task_id=\"%s\", block=false)\n"+
				"  - Stop: TaskStop(task_id=\"%s\")\n", taskID, taskID, taskID)

			if info.Output != "" {
				output += fmt.Sprintf("\nCurrent output:\n%s", info.Output)
			}

			return ui.ToolResult{
				Success: true,
				Output:  output,
				Metadata: ui.ResultMetadata{
					Title:    t.Name(),
					Icon:     t.Icon(),
					Subtitle: fmt.Sprintf("%s: still running", taskID),
					Duration: time.Since(start),
				},
			}
		}
	}

	// Get task status
	info := bgTask.GetStatus()
	statusStr := formatStatusString(info)
	output := formatTaskOutput(info, statusStr)

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
			Duration: time.Since(start),
		},
	}
}

// formatStatusString converts task status to a display string
func formatStatusString(info task.TaskInfo) string {
	switch info.Status {
	case task.StatusRunning:
		return "running"
	case task.StatusCompleted:
		return "completed"
	case task.StatusFailed:
		if info.Type == task.TaskTypeBash && info.ExitCode != 0 {
			return fmt.Sprintf("failed (exit code: %d)", info.ExitCode)
		}
		return "failed"
	case task.StatusKilled:
		return "killed"
	default:
		return "unknown"
	}
}

// formatTaskOutput builds the output string based on task type
func formatTaskOutput(info task.TaskInfo, status string) string {
	switch info.Type {
	case task.TaskTypeAgent:
		return fmt.Sprintf("Agent: %s\nStatus: %s\nTurns: %d\nTokens: %d\n",
			info.AgentName, status, info.TurnCount, info.TokenUsage)
	case task.TaskTypeBash:
		output := fmt.Sprintf("Task ID: %s\nStatus: %s\nPID: %d\n", info.ID, status, info.PID)
		if info.Command != "" {
			output += fmt.Sprintf("Command: %s\n", info.Command)
		}
		return output
	default:
		return fmt.Sprintf("Task ID: %s\nStatus: %s\n", info.ID, status)
	}
}

func init() {
	t := &TaskOutputTool{}
	Register(t)
	DefaultRegistry.RegisterAlias("AgentOutput", t)
}

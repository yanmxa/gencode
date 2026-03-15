package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoListTool lists all tracked tasks
type TodoListTool struct{}

func (t *TodoListTool) Name() string        { return "TaskList" }
func (t *TodoListTool) Description() string { return "List all tracked tasks" }
func (t *TodoListTool) Icon() string        { return "📋" }

func (t *TodoListTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	// Reload from disk to pick up changes from other processes
	DefaultTodoStore.ReloadFromDisk()

	tasks := DefaultTodoStore.List()

	if len(tasks) == 0 {
		return ui.ToolResult{
			Success: true,
			Output:  "No tasks found.",
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: "0 tasks",
			},
		}
	}

	// Build compact output: one line per task with ID, status, owner.
	// Subject is omitted — the full task list is visible in the todo panel.
	// LLM can use TaskGet(taskId) for full details.
	var sb strings.Builder
	completed := 0
	for _, task := range tasks {
		if task.Status == TodoStatusCompleted {
			completed++
		}
		line := fmt.Sprintf("#%s [%s]", task.ID, task.Status)
		if task.Owner != "" {
			line += fmt.Sprintf(" owner:%s", task.Owner)
		}
		sb.WriteString(line + "\n")
	}

	subtitle := fmt.Sprintf("%d/%d done", completed, len(tasks))

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: subtitle,
		},
	}
}

// TaskIcon returns the status icon for a task.
func TaskIcon(task *TodoTask) string {
	switch task.Status {
	case TodoStatusCompleted:
		return "✓"
	case TodoStatusInProgress:
		return "⠋"
	default:
		if DefaultTodoStore.IsBlocked(task.ID) {
			return "▸"
		}
		return "☐"
	}
}

func init() {
	Register(&TodoListTool{})
}

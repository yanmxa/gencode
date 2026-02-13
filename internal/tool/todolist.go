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

	var sb strings.Builder
	completed := 0
	for _, task := range tasks {
		if task.Status == TodoStatusCompleted {
			completed++
		}

		icon := taskIcon(task)
		line := fmt.Sprintf("%s #%s: %s [%s]", icon, task.ID, task.Subject, task.Status)
		if task.Owner != "" {
			line += fmt.Sprintf(" (owner: %s)", task.Owner)
		}
		if openBlockers := DefaultTodoStore.OpenBlockers(task.ID); len(openBlockers) > 0 {
			line += fmt.Sprintf(" [blocked by: %s]", strings.Join(openBlockers, ", "))
		}
		sb.WriteString(line + "\n")
	}

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%d/%d completed", completed, len(tasks)),
		},
	}
}

// taskIcon returns the status icon for a task
func taskIcon(task *TodoTask) string {
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

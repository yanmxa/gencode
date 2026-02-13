package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoGetTool retrieves a task by ID
type TodoGetTool struct{}

func (t *TodoGetTool) Name() string        { return "TaskGet" }
func (t *TodoGetTool) Description() string { return "Retrieve task details by ID" }
func (t *TodoGetTool) Icon() string        { return "📋" }

func (t *TodoGetTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	taskID, _ := params["taskId"].(string)
	if taskID == "" {
		return ui.NewErrorResult(t.Name(), "taskId is required")
	}

	task, ok := DefaultTodoStore.Get(taskID)
	if !ok {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("task %s not found", taskID))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Task #%s: %s\n", task.ID, task.Subject)
	fmt.Fprintf(&sb, "Status: %s\n", task.Status)
	if task.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", task.Description)
	}
	if task.ActiveForm != "" {
		fmt.Fprintf(&sb, "Active form: %s\n", task.ActiveForm)
	}
	if task.Owner != "" {
		fmt.Fprintf(&sb, "Owner: %s\n", task.Owner)
	}
	if len(task.Blocks) > 0 {
		fmt.Fprintf(&sb, "Blocks: %s\n", strings.Join(task.Blocks, ", "))
	}
	if openBlockers := DefaultTodoStore.OpenBlockers(task.ID); len(openBlockers) > 0 {
		fmt.Fprintf(&sb, "Blocked by (open): %s\n", strings.Join(openBlockers, ", "))
	}

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("#%s %s", task.ID, task.Subject),
		},
	}
}

func init() {
	Register(&TodoGetTool{})
}

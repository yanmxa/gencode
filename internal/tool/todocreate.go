package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoCreateTool creates a new tracked task
type TodoCreateTool struct{}

func (t *TodoCreateTool) Name() string        { return "TaskCreate" }
func (t *TodoCreateTool) Description() string { return "Create a task to track progress" }
func (t *TodoCreateTool) Icon() string        { return "📋" }

func (t *TodoCreateTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	subject, _ := params["subject"].(string)
	if subject == "" {
		return ui.NewErrorResult(t.Name(), "subject is required")
	}

	description, _ := params["description"].(string)
	if description == "" {
		return ui.NewErrorResult(t.Name(), "description is required")
	}

	activeForm, _ := params["activeForm"].(string)
	metadata, _ := params["metadata"].(map[string]any)

	task := DefaultTodoStore.Create(subject, description, activeForm, metadata)

	return ui.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Task #%s created: %s", task.ID, task.Subject),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: task.Subject,
		},
	}
}

func init() {
	Register(&TodoCreateTool{})
}

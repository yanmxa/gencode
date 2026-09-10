package tasktools

import (
	"context"
	"fmt"

	"github.com/genai-io/san/internal/todo"
	"github.com/genai-io/san/internal/tool"
	"github.com/genai-io/san/internal/tool/toolresult"
)

// TaskCreateTool creates a new plan task.
type TaskCreateTool struct{}

func (t *TaskCreateTool) Name() string        { return "TaskCreate" }
func (t *TaskCreateTool) Description() string { return "Create a task to track progress" }
func (t *TaskCreateTool) Icon() string        { return "📋" }

func (t *TaskCreateTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	subject := tool.GetString(params, "subject")
	if subject == "" {
		return toolresult.NewErrorResult(t.Name(), "subject is required")
	}

	description := tool.GetString(params, "description")
	if description == "" {
		return toolresult.NewErrorResult(t.Name(), "description is required")
	}

	activeForm := tool.GetString(params, "activeForm")
	metadata, _ := params["metadata"].(map[string]any)

	task := todo.Default().Create(subject, description, activeForm, metadata)

	// Set dependencies if provided
	if ids := parseStringSlice(params["addBlockedBy"]); len(ids) > 0 {
		todo.Default().Update(task.ID, todo.WithAddBlockedBy(ids))
	}

	return toolresult.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Task #%s created: %s", task.ID, task.Subject),
		Metadata: toolresult.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: task.Subject,
		},
	}
}

func init() {
	tool.Register(&TaskCreateTool{})
}

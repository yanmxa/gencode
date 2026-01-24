package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoWriteTool manages the task list for tracking progress
type TodoWriteTool struct{}

// NewTodoWriteTool creates a new TodoWriteTool
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) Name() string {
	return "TodoWrite"
}

func (t *TodoWriteTool) Description() string {
	return "Create and manage a structured task list. Use this to track progress on multi-step tasks."
}

func (t *TodoWriteTool) Icon() string {
	return "📋"
}

func (t *TodoWriteTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	// Parse the todos parameter
	todosRaw, ok := params["todos"]
	if !ok {
		return ui.NewErrorResult("TodoWrite", "missing required parameter: todos")
	}

	// Convert to JSON and back to properly parse the structure
	todosJSON, err := json.Marshal(todosRaw)
	if err != nil {
		return ui.NewErrorResult("TodoWrite", "invalid todos format: "+err.Error())
	}

	var todos []ui.TodoItem
	if err := json.Unmarshal(todosJSON, &todos); err != nil {
		return ui.NewErrorResult("TodoWrite", "failed to parse todos: "+err.Error())
	}

	// Validate each todo item
	for i, todo := range todos {
		if todo.Content == "" {
			return ui.NewErrorResult("TodoWrite", fmt.Sprintf("todo[%d]: content is required", i))
		}
		if todo.Status == "" {
			return ui.NewErrorResult("TodoWrite", fmt.Sprintf("todo[%d]: status is required", i))
		}
		if todo.Status != "pending" && todo.Status != "in_progress" && todo.Status != "completed" {
			return ui.NewErrorResult("TodoWrite", fmt.Sprintf("todo[%d]: invalid status '%s' (must be pending, in_progress, or completed)", i, todo.Status))
		}
		if todo.ActiveForm == "" {
			return ui.NewErrorResult("TodoWrite", fmt.Sprintf("todo[%d]: activeForm is required", i))
		}
	}

	// Count todos by status
	pending, inProgress, completed := 0, 0, 0
	for _, todo := range todos {
		switch todo.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "completed":
			completed++
		}
	}

	// Create output summary for LLM
	output := fmt.Sprintf("Todo list updated: %d pending, %d in progress, %d completed",
		pending, inProgress, completed)

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:     "TodoWrite",
			Icon:      "📋",
			Subtitle:  fmt.Sprintf("%d tasks", len(todos)),
			ItemCount: len(todos),
		},
		TodoItems: todos,
	}
}

func init() {
	Register(NewTodoWriteTool())
}

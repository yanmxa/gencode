package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoUpdateTool updates a task's status or details
type TodoUpdateTool struct{}

func (t *TodoUpdateTool) Name() string        { return "TaskUpdate" }
func (t *TodoUpdateTool) Description() string { return "Update task status or details" }
func (t *TodoUpdateTool) Icon() string        { return "📋" }

func (t *TodoUpdateTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	taskID, _ := params["taskId"].(string)
	if taskID == "" {
		return ui.NewErrorResult(t.Name(), "taskId is required")
	}

	// Handle deletion separately
	if status, _ := params["status"].(string); status == TodoStatusDeleted {
		if err := DefaultTodoStore.Delete(taskID); err != nil {
			return ui.NewErrorResult(t.Name(), err.Error())
		}
		return ui.ToolResult{
			Success: true,
			Output:  fmt.Sprintf("Task #%s deleted", taskID),
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("#%s deleted", taskID),
			},
		}
	}

	opts, statusChange, err := buildUpdateOptions(params)
	if err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}

	if len(opts) == 0 {
		return ui.NewErrorResult(t.Name(), "no updates specified")
	}

	if err := DefaultTodoStore.Update(taskID, opts...); err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}

	subtitle := fmt.Sprintf("#%s", taskID)
	if statusChange != "" {
		subtitle += " " + statusChange
	}

	return ui.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Updated task #%s", taskID),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: subtitle,
		},
	}
}

// buildUpdateOptions extracts update options from params, returns options, status change, and error
func buildUpdateOptions(params map[string]any) ([]UpdateOption, string, error) {
	var opts []UpdateOption
	var statusChange string

	if status, ok := params["status"].(string); ok && status != "" {
		switch status {
		case TodoStatusPending, TodoStatusInProgress, TodoStatusCompleted:
			opts = append(opts, WithStatus(status))
			statusChange = status
		default:
			return nil, "", fmt.Errorf("invalid status: %s (must be pending, in_progress, completed, or deleted)", status)
		}
	}

	if subject, ok := params["subject"].(string); ok && subject != "" {
		opts = append(opts, WithSubject(subject))
	}
	if description, ok := params["description"].(string); ok && description != "" {
		opts = append(opts, WithDescription(description))
	}
	if activeForm, ok := params["activeForm"].(string); ok && activeForm != "" {
		opts = append(opts, WithActiveForm(activeForm))
	}
	if owner, ok := params["owner"].(string); ok && owner != "" {
		opts = append(opts, WithOwner(owner))
	}
	if metadata, ok := params["metadata"].(map[string]any); ok {
		opts = append(opts, WithMetadata(metadata))
	}
	if ids := parseStringSlice(params["addBlocks"]); len(ids) > 0 {
		opts = append(opts, WithAddBlocks(ids))
	}
	if ids := parseStringSlice(params["addBlockedBy"]); len(ids) > 0 {
		opts = append(opts, WithAddBlockedBy(ids))
	}

	return opts, statusChange, nil
}

// parseStringSlice converts an interface{} to []string, handling both
// []string and []interface{} (from JSON unmarshaling)
func parseStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		var result []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// Try to parse as JSON array
		var result []string
		if err := json.Unmarshal([]byte(val), &result); err == nil {
			return result
		}
		return []string{val}
	}
	return nil
}

func init() {
	Register(&TodoUpdateTool{})
}

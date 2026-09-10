package session

import (
	"github.com/genai-io/san/internal/session/transcript"
	"github.com/genai-io/san/internal/todo"
)

// planTaskViewsFromTasks adapts the todo domain model at the session
// boundary. The transcript package deliberately owns only its persistence DTO.
func planTaskViewsFromTasks(tasks []todo.Task) []transcript.PlanTaskView {
	out := make([]transcript.PlanTaskView, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, transcript.PlanTaskView{
			ID:              task.ID,
			Subject:         task.Subject,
			Description:     task.Description,
			ActiveForm:      task.ActiveForm,
			Status:          task.Status,
			Owner:           task.Owner,
			Metadata:        cloneTaskMetadata(task.Metadata),
			Blocks:          append([]string(nil), task.Blocks...),
			BlockedBy:       append([]string(nil), task.BlockedBy...),
			CreatedAt:       task.CreatedAt,
			UpdatedAt:       task.UpdatedAt,
			StatusChangedAt: task.StatusChangedAt,
		})
	}
	return out
}

func planTasksFromViews(tasks []transcript.PlanTaskView) []todo.Task {
	out := make([]todo.Task, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, todo.Task{
			ID:              task.ID,
			Subject:         task.Subject,
			Description:     task.Description,
			ActiveForm:      task.ActiveForm,
			Status:          task.Status,
			Owner:           task.Owner,
			Metadata:        cloneTaskMetadata(task.Metadata),
			Blocks:          append([]string(nil), task.Blocks...),
			BlockedBy:       append([]string(nil), task.BlockedBy...),
			CreatedAt:       task.CreatedAt,
			UpdatedAt:       task.UpdatedAt,
			StatusChangedAt: task.StatusChangedAt,
		})
	}
	return out
}

func cloneTaskMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

package session

import (
	"reflect"
	"testing"
	"time"

	"github.com/genai-io/san/internal/todo"
)

func TestTaskViewRoundTripPreservesDomainFields(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 32, 0, 0, time.UTC)
	tasks := []todo.Task{{
		ID:              "1",
		Subject:         "Refactor",
		Description:     "Move projection helpers",
		ActiveForm:      "Refactoring",
		Status:          todo.StatusInProgress,
		Owner:           "main",
		Metadata:        map[string]any{"source": "review"},
		Blocks:          []string{"2"},
		BlockedBy:       []string{"3"},
		CreatedAt:       now,
		UpdatedAt:       now,
		StatusChangedAt: now,
	}}

	got := planTasksFromViews(planTaskViewsFromTasks(tasks))
	if !reflect.DeepEqual(got, tasks) {
		t.Fatalf("task roundtrip mismatch:\n got: %+v\nwant: %+v", got, tasks)
	}
}

func TestTaskViewAdapterDoesNotAliasMutableFields(t *testing.T) {
	tasks := []todo.Task{{
		ID:       "1",
		Metadata: map[string]any{"owner": "main"},
		Blocks:   []string{"2"},
	}}

	views := planTaskViewsFromTasks(tasks)
	views[0].Metadata["owner"] = "worker"
	views[0].Blocks[0] = "3"

	if tasks[0].Metadata["owner"] != "main" || tasks[0].Blocks[0] != "2" {
		t.Fatalf("adapter aliased domain state: tasks=%+v views=%+v", tasks, views)
	}
}

package tool

import (
	"context"
	"strings"
	"testing"
)

func useTestTodoStore(t *testing.T) *TodoStore {
	t.Helper()
	prev := DefaultTodoStore
	store := NewTodoStore()
	if err := store.SetStorageDir(t.TempDir()); err != nil {
		t.Fatalf("SetStorageDir(): %v", err)
	}
	DefaultTodoStore = store
	t.Cleanup(func() { DefaultTodoStore = prev })
	return store
}

func TestTodoGetTool_ShowsOwnerAndOpenBlockers(t *testing.T) {
	store := useTestTodoStore(t)

	blocker := store.Create("Blocker", "finish first", "blocking", nil)
	blocked := store.Create("Blocked", "waits on blocker", "waiting", nil)
	if err := store.Update(blocked.ID, WithOwner("Explore"), WithAddBlockedBy([]string{blocker.ID})); err != nil {
		t.Fatalf("Update(blocked): %v", err)
	}

	result := (&TodoGetTool{}).Execute(context.Background(), map[string]any{
		"taskId": blocked.ID,
	}, "")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Owner: Explore") {
		t.Fatalf("expected owner in output, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "Blocked by (open): "+blocker.ID) {
		t.Fatalf("expected open blocker in output, got %q", result.Output)
	}
}

func TestTodoUpdateTool_ParsesJSONBlockedByAndPersistsFields(t *testing.T) {
	store := useTestTodoStore(t)

	blocker := store.Create("Blocker", "must finish first", "blocking", nil)
	task := store.Create("Implement", "write tests", "writing", nil)

	result := (&TodoUpdateTool{}).Execute(context.Background(), map[string]any{
		"taskId":       task.ID,
		"status":       TodoStatusInProgress,
		"owner":        "Plan",
		"description":  "write more tests",
		"addBlockedBy": `["` + blocker.ID + `"]`,
	}, "")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Metadata.Subtitle != "#"+task.ID+" "+TodoStatusInProgress {
		t.Fatalf("unexpected subtitle %q", result.Metadata.Subtitle)
	}

	updated, ok := store.Get(task.ID)
	if !ok {
		t.Fatal("expected updated task to exist")
	}
	if updated.Status != TodoStatusInProgress {
		t.Fatalf("status = %q, want %q", updated.Status, TodoStatusInProgress)
	}
	if updated.Owner != "Plan" {
		t.Fatalf("owner = %q, want %q", updated.Owner, "Plan")
	}
	if updated.Description != "write more tests" {
		t.Fatalf("description = %q, want %q", updated.Description, "write more tests")
	}
	if len(updated.BlockedBy) != 1 || updated.BlockedBy[0] != blocker.ID {
		t.Fatalf("blockedBy = %#v, want [%q]", updated.BlockedBy, blocker.ID)
	}
}

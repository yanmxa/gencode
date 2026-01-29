package task

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := m.Create(cmd, "echo test", "Test task", ctx, cancel)

	if task.ID == "" {
		t.Error("task ID should not be empty")
	}

	retrieved, ok := m.Get(task.ID)
	if !ok {
		t.Error("should find created task")
	}
	if retrieved.ID != task.ID {
		t.Error("retrieved task should match created task")
	}
}

func TestManager_GetNotFound(t *testing.T) {
	m := NewManager()

	_, ok := m.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent task")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple tasks
	for i := 0; i < 3; i++ {
		cmd := exec.CommandContext(ctx, "echo", "test")
		cmd.Start()
		m.Create(cmd, "echo test", "Test task", ctx, cancel)
	}

	tasks := m.List()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestManager_ListRunning(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create 3 tasks
	var tasks []*Task
	for i := 0; i < 3; i++ {
		cmd := exec.CommandContext(ctx, "echo", "test")
		cmd.Start()
		task := m.Create(cmd, "echo test", "Test task", ctx, cancel)
		tasks = append(tasks, task)
	}

	// Complete one task
	tasks[0].Complete(0, nil)

	running := m.ListRunning()
	if len(running) != 2 {
		t.Errorf("expected 2 running tasks, got %d", len(running))
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := m.Create(cmd, "echo test", "Test task", ctx, cancel)
	taskID := task.ID

	m.Remove(taskID)

	_, ok := m.Get(taskID)
	if ok {
		t.Error("task should be removed")
	}
}

func TestManager_Cleanup(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create and complete a task
	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := m.Create(cmd, "echo test", "Test task", ctx, cancel)
	task.Complete(0, nil)

	// Set EndTime to past
	task.mu.Lock()
	task.EndTime = time.Now().Add(-2 * time.Hour)
	task.mu.Unlock()

	// Cleanup tasks older than 1 hour
	m.Cleanup(time.Hour)

	_, ok := m.Get(task.ID)
	if ok {
		t.Error("old completed task should be cleaned up")
	}
}

func TestManager_CleanupKeepsRecent(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := m.Create(cmd, "echo test", "Test task", ctx, cancel)
	task.Complete(0, nil)

	// Cleanup with 1 hour threshold - task just completed so should be kept
	m.Cleanup(time.Hour)

	_, ok := m.Get(task.ID)
	if !ok {
		t.Error("recently completed task should not be cleaned up")
	}
}

func TestManager_CleanupKeepsRunning(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := m.Create(cmd, "echo test", "Test task", ctx, cancel)

	// Don't complete, keep it running
	m.Cleanup(0) // Cleanup all old tasks

	_, ok := m.Get(task.ID)
	if !ok {
		t.Error("running task should not be cleaned up")
	}
}

func TestManager_GenerateUniqueIDs(t *testing.T) {
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		cmd := exec.CommandContext(ctx, "echo", "test")
		cmd.Start()
		task := m.Create(cmd, "echo test", "Test task", ctx, cancel)

		if ids[task.ID] {
			t.Errorf("duplicate ID generated: %s", task.ID)
		}
		ids[task.ID] = true
	}
}

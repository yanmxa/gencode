package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Manager manages background tasks
type Manager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// DefaultManager is the global default task manager
var DefaultManager = NewManager()

// NewManager creates a new task manager
func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*Task),
	}
}

// Create creates and registers a new task
func (m *Manager) Create(cmd *exec.Cmd, command, description string, ctx context.Context, cancel context.CancelFunc) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateID()
	task := NewTask(id, command, description, cmd, ctx, cancel)

	m.tasks[id] = task

	return task
}

// generateID creates a short random ID
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Get retrieves a task by ID
func (m *Manager) Get(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

// List returns all tasks
func (m *Manager) List() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// ListRunning returns all running tasks
func (m *Manager) ListRunning() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0)
	for _, t := range m.tasks {
		if t.IsRunning() {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// Kill terminates a task by ID
func (m *Manager) Kill(id string) error {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	if !task.IsRunning() {
		return fmt.Errorf("task already completed: %s", id)
	}

	// Cancel the context first
	if task.Cancel != nil {
		task.Cancel()
	}

	// Try graceful termination first (SIGTERM to process group)
	if task.PID > 0 {
		// Use negative PID to kill the process group
		if err := syscall.Kill(-task.PID, syscall.SIGTERM); err != nil {
			// If SIGTERM fails, try SIGKILL
			syscall.Kill(-task.PID, syscall.SIGKILL)
		}
	}

	// Wait for graceful exit
	done := make(chan struct{})
	go func() {
		for task.IsRunning() {
			time.Sleep(100 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
		// Already terminated
	case <-time.After(2 * time.Second):
		// Force kill
		if task.PID > 0 {
			syscall.Kill(-task.PID, syscall.SIGKILL)
		}
		// Wait a bit more
		time.Sleep(500 * time.Millisecond)
	}

	// Mark as killed if still running
	if task.IsRunning() {
		task.Kill()
	}

	return nil
}

// Remove removes a completed task from the manager
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
}

// Cleanup removes all completed tasks older than maxAge
func (m *Manager) Cleanup(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, task := range m.tasks {
		if !task.IsRunning() && now.Sub(task.EndTime) > maxAge {
			delete(m.tasks, id)
		}
	}
}

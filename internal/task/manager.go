package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Manager manages background tasks
type Manager struct {
	mu    sync.RWMutex
	tasks map[string]BackgroundTask
}

// DefaultManager is the global default task manager
var DefaultManager = NewManager()

// NewManager creates a new task manager
func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]BackgroundTask),
	}
}

// CreateBashTask creates and registers a new bash task
func (m *Manager) CreateBashTask(cmd *exec.Cmd, command, description string, ctx context.Context, cancel context.CancelFunc) *BashTask {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateID()
	task := NewBashTask(id, command, description, cmd, ctx, cancel)

	m.tasks[id] = task

	return task
}

// Create is an alias for CreateBashTask for backward compatibility
// Deprecated: Use CreateBashTask instead
func (m *Manager) Create(cmd *exec.Cmd, command, description string, ctx context.Context, cancel context.CancelFunc) *BashTask {
	return m.CreateBashTask(cmd, command, description, ctx, cancel)
}

// RegisterTask registers an existing task (used for agent tasks)
func (m *Manager) RegisterTask(task BackgroundTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.GetID()] = task
}

// generateID creates a short random ID
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateID creates a short random ID (exported for agent tasks)
func GenerateID() string {
	return generateID()
}

// Get retrieves a task by ID
func (m *Manager) Get(id string) (BackgroundTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

// GetBashTask retrieves a bash task by ID (for backward compatibility)
func (m *Manager) GetBashTask(id string) (*BashTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	bashTask, ok := task.(*BashTask)
	return bashTask, ok
}

// List returns all tasks
func (m *Manager) List() []BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]BackgroundTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// ListRunning returns all running tasks
func (m *Manager) ListRunning() []BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]BackgroundTask, 0)
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

	// Try graceful stop first
	if err := task.Stop(); err != nil {
		// If stop fails, try kill
		return task.Kill()
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
		if err := task.Kill(); err != nil {
			return err
		}
		// Wait a bit more
		time.Sleep(500 * time.Millisecond)
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
		info := task.GetStatus()
		if !task.IsRunning() && now.Sub(info.EndTime) > maxAge {
			delete(m.tasks, id)
		}
	}
}

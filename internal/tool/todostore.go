package tool

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// TodoTask represents a tracked task
type TodoTask struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Status      string         `json:"status"`
	Owner       string         `json:"owner,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Blocks      []string       `json:"blocks,omitempty"`
	BlockedBy   []string       `json:"blockedBy,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// Task status constants
const (
	TodoStatusPending    = "pending"
	TodoStatusInProgress = "in_progress"
	TodoStatusCompleted  = "completed"
	TodoStatusDeleted    = "deleted"
)

// TodoStore is a thread-safe in-memory task store
type TodoStore struct {
	mu     sync.RWMutex
	tasks  map[string]*TodoTask
	nextID int
}

// NewTodoStore creates a new TodoStore
func NewTodoStore() *TodoStore {
	return &TodoStore{
		tasks:  make(map[string]*TodoTask),
		nextID: 1,
	}
}

// DefaultTodoStore is the global task store singleton
var DefaultTodoStore = NewTodoStore()

// Create adds a new task and returns it
func (s *TodoStore) Create(subject, description, activeForm string, metadata map[string]any) *TodoTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%d", s.nextID)
	s.nextID++

	now := time.Now()
	task := &TodoTask{
		ID:          id,
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      TodoStatusPending,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.tasks[id] = task
	return task
}

// Get retrieves a task by ID
func (s *TodoStore) Get(id string) (*TodoTask, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok || task.Status == TodoStatusDeleted {
		return nil, false
	}
	return task, true
}

// Update modifies an existing task. Returns error if task not found.
func (s *TodoStore) Update(id string, opts ...UpdateOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	for _, opt := range opts {
		opt(task)
	}

	task.UpdatedAt = time.Now()
	return nil
}

// List returns all non-deleted tasks sorted by ID
func (s *TodoStore) List() []*TodoTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*TodoTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.Status != TodoStatusDeleted {
			tasks = append(tasks, task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	return tasks
}

// IsBlocked returns true if the task has any uncompleted blockers
func (s *TodoStore) IsBlocked(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok || task.Status == TodoStatusDeleted {
		return false
	}

	for _, blockerID := range task.BlockedBy {
		blocker, ok := s.tasks[blockerID]
		if ok && blocker.Status != TodoStatusCompleted && blocker.Status != TodoStatusDeleted {
			return true
		}
	}
	return false
}

// OpenBlockers returns IDs of uncompleted tasks that block the given task
func (s *TodoStore) OpenBlockers(id string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok || task.Status == TodoStatusDeleted {
		return nil
	}

	var open []string
	for _, blockerID := range task.BlockedBy {
		blocker, ok := s.tasks[blockerID]
		if ok && blocker.Status != TodoStatusCompleted && blocker.Status != TodoStatusDeleted {
			open = append(open, blockerID)
		}
	}
	return open
}

// Delete marks a task as deleted
func (s *TodoStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	task.Status = TodoStatusDeleted
	task.UpdatedAt = time.Now()
	return nil
}

// Reset clears all tasks (for new sessions)
func (s *TodoStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks = make(map[string]*TodoTask)
	s.nextID = 1
}

// Export returns a snapshot of all tasks (including deleted) for session persistence
func (s *TodoStore) Export() []TodoTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]TodoTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, *t)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// Import restores tasks from a snapshot (used when loading a session)
func (s *TodoStore) Import(tasks []TodoTask) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks = make(map[string]*TodoTask, len(tasks))
	s.nextID = 1
	for i := range tasks {
		t := tasks[i]
		s.tasks[t.ID] = &t
		// Parse ID to maintain nextID counter
		var idNum int
		if _, err := fmt.Sscanf(t.ID, "%d", &idNum); err == nil && idNum >= s.nextID {
			s.nextID = idNum + 1
		}
	}
}

// UpdateOption is a functional option for updating a task
type UpdateOption func(*TodoTask)

// WithStatus sets the task status
func WithStatus(status string) UpdateOption {
	return func(t *TodoTask) {
		t.Status = status
	}
}

// WithSubject sets the task subject
func WithSubject(subject string) UpdateOption {
	return func(t *TodoTask) {
		t.Subject = subject
	}
}

// WithDescription sets the task description
func WithDescription(description string) UpdateOption {
	return func(t *TodoTask) {
		t.Description = description
	}
}

// WithActiveForm sets the task activeForm
func WithActiveForm(activeForm string) UpdateOption {
	return func(t *TodoTask) {
		t.ActiveForm = activeForm
	}
}

// WithOwner sets the task owner
func WithOwner(owner string) UpdateOption {
	return func(t *TodoTask) {
		t.Owner = owner
	}
}

// WithMetadata merges metadata (nil values delete keys)
func WithMetadata(metadata map[string]any) UpdateOption {
	return func(t *TodoTask) {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		for k, v := range metadata {
			if v == nil {
				delete(t.Metadata, k)
			} else {
				t.Metadata[k] = v
			}
		}
	}
}

// WithAddBlocks adds task IDs that this task blocks
func WithAddBlocks(ids []string) UpdateOption {
	return func(t *TodoTask) {
		t.Blocks = appendUnique(t.Blocks, ids)
	}
}

// WithAddBlockedBy adds task IDs that block this task
func WithAddBlockedBy(ids []string) UpdateOption {
	return func(t *TodoTask) {
		t.BlockedBy = appendUnique(t.BlockedBy, ids)
	}
}

// appendUnique appends ids to slice, skipping duplicates
func appendUnique(slice, ids []string) []string {
	existing := make(map[string]bool, len(slice))
	for _, id := range slice {
		existing[id] = true
	}
	for _, id := range ids {
		if !existing[id] {
			slice = append(slice, id)
			existing[id] = true
		}
	}
	return slice
}

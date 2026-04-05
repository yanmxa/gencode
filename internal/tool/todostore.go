package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// TodoTask represents a tracked task
type TodoTask struct {
	ID              string         `json:"id"`
	Subject         string         `json:"subject"`
	Description     string         `json:"description"`
	ActiveForm      string         `json:"activeForm,omitempty"`
	Status          string         `json:"status"`
	Owner           string         `json:"owner,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Blocks          []string       `json:"blocks"`
	BlockedBy       []string       `json:"blockedBy"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	StatusChangedAt time.Time      `json:"statusChangedAt"` // when status last changed (for elapsed time display)
}

// Task status constants
const (
	TodoStatusPending    = "pending"
	TodoStatusInProgress = "in_progress"
	TodoStatusCompleted  = "completed"
	TodoStatusDeleted    = "deleted"
)

// TodoStore is a thread-safe task store with optional disk persistence.
// When a storageDir is set, each task is persisted as {id}.json.
type TodoStore struct {
	mu         sync.RWMutex
	tasks      map[string]*TodoTask
	nextID     int
	storageDir string    // empty = in-memory only
	lastDirMod time.Time // last known dir mtime, for change detection in ReloadFromDisk
}

// NewTodoStore creates a new in-memory TodoStore
func NewTodoStore() *TodoStore {
	return &TodoStore{
		tasks:  make(map[string]*TodoTask),
		nextID: 1,
	}
}

// DefaultTodoStore is the global task store singleton
var DefaultTodoStore = NewTodoStore()

// SetStorageDir sets the directory for disk persistence and loads existing tasks.
// If dir is empty, the store operates in memory-only mode.
func (s *TodoStore) SetStorageDir(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.storageDir = dir
	if dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create task storage dir: %w", err)
	}

	// Create lock file
	lockPath := filepath.Join(dir, ".lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		os.WriteFile(lockPath, nil, 0o644)
	}

	// Load existing tasks from disk
	return s.loadFromDisk()
}

// loadFromDisk reads all {id}.json files from storageDir into memory.
// Must be called with s.mu held.
func (s *TodoStore) loadFromDisk() error {
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return err
	}

	s.tasks = make(map[string]*TodoTask)
	s.nextID = 1

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.storageDir, name))
		if err != nil {
			continue
		}

		var task TodoTask
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}

		normalizeTaskSlices(&task)
		s.tasks[task.ID] = &task
		var idNum int
		if _, err := fmt.Sscanf(task.ID, "%d", &idNum); err == nil && idNum >= s.nextID {
			s.nextID = idNum + 1
		}
	}

	return nil
}

// persistTask writes a single task to disk. Must be called with s.mu held.
func (s *TodoStore) persistTask(task *TodoTask) {
	if s.storageDir == "" {
		return
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(s.storageDir, task.ID+".json")
	os.WriteFile(path, data, 0o644)
}

// removeTaskFile deletes a task file from disk. Must be called with s.mu held.
func (s *TodoStore) removeTaskFile(id string) {
	if s.storageDir == "" {
		return
	}
	_ = os.Remove(filepath.Join(s.storageDir, id+".json"))
}

// Create adds a new task and returns it
func (s *TodoStore) Create(subject, description, activeForm string, metadata map[string]any) *TodoTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%d", s.nextID)
	s.nextID++

	now := time.Now()
	task := &TodoTask{
		ID:              id,
		Subject:         subject,
		Description:     description,
		ActiveForm:      activeForm,
		Status:          TodoStatusPending,
		Metadata:        metadata,
		Blocks:          []string{},
		BlockedBy:       []string{},
		CreatedAt:       now,
		UpdatedAt:       now,
		StatusChangedAt: now,
	}

	s.tasks[id] = task
	s.persistTask(task)
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
	s.persistTask(task)
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

// Delete marks a task as deleted and removes its file from disk
func (s *TodoStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	task.Status = TodoStatusDeleted
	task.UpdatedAt = time.Now()
	s.removeTaskFile(id)
	return nil
}

// HasInProgress returns true if any task has in_progress status.
func (s *TodoStore) HasInProgress() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.tasks {
		if task.Status == TodoStatusInProgress {
			return true
		}
	}
	return false
}

// Reset clears all tasks (for new sessions)
func (s *TodoStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove all task files from disk
	if s.storageDir != "" {
		for id := range s.tasks {
			_ = os.Remove(filepath.Join(s.storageDir, id+".json"))
		}
	}

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
		normalizeTaskSlices(&t)
		s.tasks[t.ID] = &t
		var idNum int
		if _, err := fmt.Sscanf(t.ID, "%d", &idNum); err == nil && idNum >= s.nextID {
			s.nextID = idNum + 1
		}
		s.persistTask(&t)
	}
}

// GetStorageDir returns the current storage directory.
func (s *TodoStore) GetStorageDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageDir
}

// ReloadFromDisk re-reads all task files from the storage directory.
// This picks up changes made by other processes (e.g., background agents).
// No-op if no storage directory is configured or if directory hasn't been modified.
func (s *TodoStore) ReloadFromDisk() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storageDir == "" {
		return
	}

	// Check if any task file has been modified since last reload.
	// We can't rely on directory mtime alone because modifying existing
	// files in-place doesn't update the directory mtime on macOS/most
	// filesystems.
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return
	}

	changed := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(s.lastDirMod) {
			changed = true
			break
		}
	}

	if !changed && !s.lastDirMod.IsZero() {
		return
	}

	s.lastDirMod = time.Now()
	_ = s.loadFromDisk()
}

// UpdateOption is a functional option for updating a task
type UpdateOption func(*TodoTask)

// WithStatus sets the task status and records the status change timestamp.
func WithStatus(status string) UpdateOption {
	return func(t *TodoTask) {
		if t.Status != status {
			t.StatusChangedAt = time.Now()
		}
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

// normalizeTaskSlices ensures Blocks and BlockedBy are non-nil slices.
func normalizeTaskSlices(t *TodoTask) {
	if t.Blocks == nil {
		t.Blocks = []string{}
	}
	if t.BlockedBy == nil {
		t.BlockedBy = []string{}
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

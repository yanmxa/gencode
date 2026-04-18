package orchestration

import "sync"

// Service is the public contract for the orchestration module.
// It tracks background worker state, batches, pending messages, and
// produces coordinator decision snapshots.
type Service interface {
	// tracking
	RecordLaunch(launch Launch)
	RecordCompletion(taskID, status, agentID string)
	UpdateBatch(batch Batch)

	// snapshot
	Snapshot(taskID, agentID, status string, notificationCount int) (*Snapshot, bool)
	SnapshotBatchForTask(taskID string) (*Batch, bool)

	// messaging
	QueuePendingMessage(taskID, msg string) bool
	DrainPendingMessages(taskID, agentID string) []string
	PendingMessageCount(taskID, agentID string) int

	// resolution
	ResolveTaskID(agentID string) (string, bool)

	// lifecycle
	Reset()
}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

// Initialize sets up the orchestration singleton. Call once at startup.
func Initialize() {
	s := newStore()
	mu.Lock()
	instance = s
	DefaultStore = s
	mu.Unlock()
}

// Default returns the orchestration Service singleton.
// Panics if not initialized.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("orchestration: not initialized")
	}
	return s
}

// SetDefault replaces the singleton (for tests).
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetSingleton clears the singleton (for tests).
func ResetSingleton() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}

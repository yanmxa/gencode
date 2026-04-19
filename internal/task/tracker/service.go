package tracker

import "sync"

// Service is the public contract for the tracker module.
type Service interface {
	// CRUD
	Create(subject, description, activeForm string, metadata map[string]any) *Task
	Get(id string) (*Task, bool)
	Update(id string, opts ...UpdateOption) error
	Delete(id string) error
	List() []*Task

	// query
	IsBlocked(id string) bool
	OpenBlockers(id string) []string
	HasInProgress() bool
	AllDone() bool
	FindByMetadata(key, want string) *Task

	// persistence
	SetStorageDir(dir string) error
	GetStorageDir() string
	ReloadFromDisk()
	Export() []Task
	Import(tasks []Task)

	// lifecycle
	Reset()
}

// Compile-time check: *Store implements Service.
var _ Service = (*Store)(nil)

// Options holds all dependencies for initialization.
type Options struct{}

// ── singleton ──────────────────────────────────────────────

var (
	svcMu    sync.RWMutex
	instance Service
)

// Initialize creates a new Store and sets it as the singleton.
func Initialize(opts Options) {
	svcMu.Lock()
	instance = NewStore()
	svcMu.Unlock()
}

// Default returns the singleton Service instance.
// Panics if not initialized.
func Default() Service {
	svcMu.RLock()
	s := instance
	svcMu.RUnlock()
	if s == nil {
		panic("tracker: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	instance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}


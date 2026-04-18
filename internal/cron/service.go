package cron

import "sync"

// Service is the public contract for the cron module.
type Service interface {
	// CRUD
	Add(job Job) error
	Remove(id string) bool
	Create(cronExpr, prompt string, recurring, durable bool) (*Job, error)
	Delete(id string) error
	List() []*Job

	// runtime
	Tick() []FiredJob // advance clock, return fired jobs

	// query
	Empty() bool

	// lifecycle
	Reset()
	SetStoragePath(path string)
	LoadDurable() error
}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

// Options configures the cron service singleton.
type Options struct {
	StoragePath string // file path for durable job persistence
}

func init() {
	mu.Lock()
	if instance == nil {
		instance = NewStore()
	}
	mu.Unlock()
}

// Initialize creates and configures the cron service singleton.
func Initialize(opts Options) {
	s := NewStore()
	if opts.StoragePath != "" {
		s.SetStoragePath(opts.StoragePath)
	}
	mu.Lock()
	instance = s
	mu.Unlock()
}

// Default returns the singleton Service instance.
// Panics if not initialized.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("cron: not initialized")
	}
	return s
}

// SetDefault sets the singleton Service instance (for tests).
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetService clears the singleton (for tests).
func ResetService() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}

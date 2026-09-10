package todo

import "sync"

// Options holds all dependencies for initialization.
type Options struct{}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance *Store
)

// Initialize creates a new Store and sets it as the singleton.
func Initialize(opts Options) {
	mu.Lock()
	instance = NewStore()
	mu.Unlock()
}

// Default returns the singleton Service instance.
// Panics if not initialized.
func Default() *Store {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("todo: not initialized")
	}
	return s
}

// SetDefaultStore replaces the package default. Intended for tests.
func SetDefaultStore(s *Store) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetDefaultStore clears the package default. Intended for tests.
func ResetDefaultStore() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}

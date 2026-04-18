package session

import "sync"

// Service is the public contract for the session module.
type Service interface {
	// identity
	ID() string              // current session ID
	SetID(id string)         // update current session ID
	TranscriptPath() string  // path to transcript file

	// summary
	GetSummary() string      // compaction summary
	SetSummary(summary string)

	// store access
	GetStore() *Store        // underlying session store (may be nil)
	SetStore(s *Store)       // replace session store

	// persistence (delegates to store)
	Save(snap *Snapshot) error
	Load(id string) (*Snapshot, error)
	LoadLatest() (*Snapshot, error)
	List() ([]*SessionMetadata, error)
	Fork(id string) (*Snapshot, error)
	SaveMemory(id, memory string) error
	LoadMemory(id string) (string, error)
}

// Compile-time check: *Setup implements Service.
var _ Service = (*Setup)(nil)

// ── singleton ──────────────────────────────────────────────

var (
	svcMu    sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := instance
	svcMu.RUnlock()
	if s == nil {
		panic("session: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	instance = s
	svcMu.Unlock()
}

// Reset clears the singleton instance. Intended for tests.
func Reset() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}

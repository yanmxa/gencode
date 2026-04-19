package session

import (
	"sync"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
)

// Service is the public contract for the session module.
type Service interface {
	// identity
	ID() string
	SetID(id string)
	TranscriptPath() string

	// summary
	GetSummary() string
	SetSummary(summary string)

	// store access
	GetStore() *Store
	SetStore(s *Store)
	EnsureStore(cwd string) error

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

// Options holds configuration for Initialize.
type Options struct {
	CWD string
}

// Initialize creates a session store and generates a fresh session ID.
func Initialize(opts Options) {
	store, err := NewStore(opts.CWD)
	if err != nil {
		log.Logger().Warn("session store initialization failed, sessions will not be persisted", zap.Error(err))
	}

	defaultSetup.mu.Lock()
	defaultSetup.SessionID = NewSessionID()
	defaultSetup.Store = store
	defaultSetup.mu.Unlock()

	SetDefault(defaultSetup)
}

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

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}

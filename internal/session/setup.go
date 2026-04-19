package session

import (
	"fmt"
	"sync"
)

// defaultSetup is the package-level session setup, initialized by Initialize().
var defaultSetup = &Setup{}

// Setup holds the initialized session infrastructure needed by the app layer.
// The exported fields (Store, SessionID, Summary) are kept for backward
// compatibility. New code should use the Service interface methods instead.
type Setup struct {
	mu sync.RWMutex

	// Exported fields — backward compatible, accessed directly by callers.
	Store     *Store
	SessionID string
	Summary   string
}

// EnsureStore lazily initializes the session store for the given cwd.
func (s *Setup) EnsureStore(cwd string) error {
	s.mu.RLock()
	if s.Store != nil {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	store, err := NewStore(cwd)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Store == nil {
		s.Store = store
	}
	return nil
}

// ── Service interface implementation ──────────────────────

// ID returns the current session ID.
func (s *Setup) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionID
}

// SetID updates the current session ID.
func (s *Setup) SetID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionID = id
}

// TranscriptPath returns the transcript file path for the current session,
// or empty string if the store is nil.
func (s *Setup) TranscriptPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Store != nil {
		return s.Store.SessionPath(s.SessionID)
	}
	return ""
}

// GetSummary returns the compaction summary.
// Named GetSummary to avoid conflict with the exported Summary field.
func (s *Setup) GetSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Summary
}

// SetSummary updates the compaction summary.
func (s *Setup) SetSummary(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Summary = summary
}

// GetStore returns the underlying session store (may be nil).
// Named GetStore to avoid conflict with the exported Store field.
func (s *Setup) GetStore() *Store {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Store
}

// SetStore replaces the session store.
func (s *Setup) SetStore(st *Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Store = st
}

// Save persists a session snapshot via the store.
func (s *Setup) Save(snap *Snapshot) error {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return fmt.Errorf("session store not initialized")
	}
	return st.Save(snap)
}

// Load loads a session by ID via the store.
func (s *Setup) Load(id string) (*Snapshot, error) {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return nil, fmt.Errorf("session store not initialized")
	}
	return st.Load(id)
}

// LoadLatest loads the most recent session via the store.
func (s *Setup) LoadLatest() (*Snapshot, error) {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return nil, fmt.Errorf("session store not initialized")
	}
	return st.GetLatest()
}

// List returns metadata for all sessions via the store.
func (s *Setup) List() ([]*SessionMetadata, error) {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return nil, fmt.Errorf("session store not initialized")
	}
	return st.List()
}

// Fork forks a session by ID via the store.
func (s *Setup) Fork(id string) (*Snapshot, error) {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return nil, fmt.Errorf("session store not initialized")
	}
	return st.Fork(id)
}

// SaveMemory persists session memory via the store.
func (s *Setup) SaveMemory(id, memory string) error {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return fmt.Errorf("session store not initialized")
	}
	return st.SaveSessionMemory(id, memory)
}

// LoadMemory loads session memory via the store.
func (s *Setup) LoadMemory(id string) (string, error) {
	s.mu.RLock()
	st := s.Store
	s.mu.RUnlock()
	if st == nil {
		return "", fmt.Errorf("session store not initialized")
	}
	return st.LoadSessionMemory(id)
}

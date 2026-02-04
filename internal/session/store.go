package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// SessionRetentionDays is how long sessions are kept before cleanup
	SessionRetentionDays = 30
)

// Store manages session file storage
type Store struct {
	mu      sync.RWMutex
	baseDir string
}

// NewStore creates a new session store
// Sessions are stored in ~/.gen/sessions/
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".gen", "sessions")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	store := &Store{baseDir: baseDir}

	// Run cleanup on startup
	go store.Cleanup()

	return store, nil
}

// Save saves a session to disk
func (s *Store) Save(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session.Metadata.ID == "" {
		session.Metadata.ID = generateSessionID()
	}
	if session.Metadata.CreatedAt.IsZero() {
		session.Metadata.CreatedAt = time.Now()
	}
	session.Metadata.UpdatedAt = time.Now()
	session.Metadata.MessageCount = len(session.Messages)

	filePath := filepath.Join(s.baseDir, session.Metadata.ID+".json")

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// Load loads a session from disk by ID
func (s *Store) Load(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.baseDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &session, nil
}

// List returns all sessions sorted by update time (newest first)
func (s *Store) List() ([]*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SessionMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	sessions := make([]*SessionMetadata, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		session, err := s.loadWithoutLock(id)
		if err != nil {
			continue // Skip invalid session files
		}
		sessions = append(sessions, &session.Metadata)
	}

	// Sort by update time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// GetLatest returns the most recently updated session
func (s *Store) GetLatest() (*Session, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	// Sessions are already sorted by update time (newest first)
	return s.Load(sessions[0].ID)
}

// Delete removes a session file
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.baseDir, id+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// Cleanup removes sessions older than SessionRetentionDays
func (s *Store) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -SessionRetentionDays)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		session, err := s.loadWithoutLock(id)
		if err != nil {
			continue
		}

		if session.Metadata.UpdatedAt.Before(cutoff) {
			filePath := filepath.Join(s.baseDir, entry.Name())
			_ = os.Remove(filePath)
		}
	}

	return nil
}

// loadWithoutLock loads a session without acquiring the lock (caller must hold lock)
func (s *Store) loadWithoutLock(id string) (*Session, error) {
	filePath := filepath.Join(s.baseDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// generateSessionID generates a unique session ID based on timestamp
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

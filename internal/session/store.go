package session

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yanmxa/gencode/internal/tool"
)

const (
	// SessionRetentionDays is how long sessions are kept before cleanup
	SessionRetentionDays = 30

	indexFileName = "sessions-index.json"
)

// Store manages session file storage scoped to a project directory.
type Store struct {
	mu         sync.RWMutex
	baseDir    string // e.g. ~/.gen/projects/<encoded-cwd>
	cwd        string // original working directory
	projectDir string // same as baseDir (kept for clarity)
}

// NewStore creates a session store scoped to the given working directory.
// Sessions are stored in ~/.gen/projects/<encoded-cwd>/.
func NewStore(cwd string) (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := filepath.Join(homeDir, ".gen", "projects", encodePath(cwd))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	store := &Store{
		baseDir:    projectDir,
		cwd:        cwd,
		projectDir: projectDir,
	}

	// Run cleanup on startup
	go store.Cleanup()

	return store, nil
}

// NewStoreWithDir creates a session store using the given directory.
// Intended for testing — does not run background cleanup.
func NewStoreWithDir(dir string) *Store {
	return &Store{
		baseDir:    dir,
		cwd:        dir,
		projectDir: dir,
	}
}

// isMessageEntry returns true for entry types that represent conversation messages.
func isMessageEntry(entryType string) bool {
	return entryType == EntryUser || entryType == EntryAssistant
}

// Save appends new entries to the JSONL session file and updates the index.
func (s *Store) Save(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if session.Metadata.ID == "" {
		session.Metadata.ID = generateSessionID()
	}
	if session.Metadata.CreatedAt.IsZero() {
		session.Metadata.CreatedAt = now
	}
	session.Metadata.UpdatedAt = now
	session.Metadata.MessageCount = len(session.Entries)

	filePath := filepath.Join(s.baseDir, session.Metadata.ID+".jsonl")

	// Count existing message entries and find the last UUID for parentUuid chaining.
	existingCount := 0
	var lastUUID string
	if f, err := os.Open(filePath); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 16*1024*1024), 16*1024*1024)
		for scanner.Scan() {
			var entry Entry
			if json.Unmarshal(scanner.Bytes(), &entry) == nil && isMessageEntry(entry.Type) {
				existingCount++
				if entry.UUID != "" {
					lastUUID = entry.UUID
				}
			}
		}
		f.Close()
	}

	// Open file for appending (create if needed).
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	gitBranch := getGitBranch(s.cwd)

	// Append new entries.
	for i := existingCount; i < len(session.Entries); i++ {
		entry := session.Entries[i]

		// Patch parentUuid for the first new entry to chain to the last existing entry.
		if i == existingCount && lastUUID != "" {
			entry.ParentUuid = &lastUUID
		}

		// Fill context fields.
		entry.SessionID = session.Metadata.ID
		if entry.Cwd == "" {
			entry.Cwd = session.Metadata.Cwd
		}
		if entry.Version == "" {
			entry.Version = AppVersion
		}
		if entry.GitBranch == "" {
			entry.GitBranch = gitBranch
		}
		entry.Timestamp = now

		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	// Always append a metadata entry (last one wins on read).
	metaEntry := Entry{
		Type:      EntryMetadata,
		SessionID: session.Metadata.ID,
		Timestamp: now,
		Metadata: &EntryMetadata_{
			Title:           session.Metadata.Title,
			Provider:        session.Metadata.Provider,
			Model:           session.Metadata.Model,
			Cwd:             session.Metadata.Cwd,
			CreatedAt:       session.Metadata.CreatedAt,
			UpdatedAt:       session.Metadata.UpdatedAt,
			MessageCount:    session.Metadata.MessageCount,
			ParentSessionID: session.Metadata.ParentSessionID,
			Tasks:           session.Tasks,
		},
	}
	if err := enc.Encode(metaEntry); err != nil {
		return fmt.Errorf("failed to write metadata entry: %w", err)
	}

	// Update the sessions index.
	s.updateIndexEntry(session, filePath)

	return nil
}

// SaveSubagent persists a subagent session under {parentSessionID}/subagents/.
// The JSONL file uses the same format as parent sessions but is marked as
// isSidechain in the index so it won't appear in the resume selector.
func (s *Store) SaveSubagent(parentSessionID string, sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if sess.Metadata.ID == "" {
		sess.Metadata.ID = "agent-" + GenerateShortID()
	}
	if sess.Metadata.CreatedAt.IsZero() {
		sess.Metadata.CreatedAt = now
	}
	sess.Metadata.UpdatedAt = now
	sess.Metadata.MessageCount = len(sess.Entries)
	sess.Metadata.ParentSessionID = parentSessionID

	// Create subagents directory: {baseDir}/{parentSessionID}/subagents/
	subagentsDir := filepath.Join(s.baseDir, parentSessionID, "subagents")
	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create subagents directory: %w", err)
	}

	filePath := filepath.Join(subagentsDir, sess.Metadata.ID+".jsonl")

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open subagent session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	// Write all entries with sidechain and agentId fields.
	for i := range sess.Entries {
		entry := sess.Entries[i]
		entry.SessionID = sess.Metadata.ID
		entry.IsSidechain = true
		if entry.Cwd == "" {
			entry.Cwd = sess.Metadata.Cwd
		}
		if entry.Version == "" {
			entry.Version = AppVersion
		}
		entry.Timestamp = now

		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	// Write metadata entry.
	metaEntry := Entry{
		Type:      EntryMetadata,
		SessionID: sess.Metadata.ID,
		Timestamp: now,
		Metadata: &EntryMetadata_{
			Title:        sess.Metadata.Title,
			Provider:     sess.Metadata.Provider,
			Model:        sess.Metadata.Model,
			Cwd:          sess.Metadata.Cwd,
			CreatedAt:    sess.Metadata.CreatedAt,
			UpdatedAt:    sess.Metadata.UpdatedAt,
			MessageCount: sess.Metadata.MessageCount,
		},
	}
	if err := enc.Encode(metaEntry); err != nil {
		return fmt.Errorf("failed to write metadata entry: %w", err)
	}

	// Update index with isSidechain = true.
	s.updateIndexEntrySidechain(sess, filePath)

	return nil
}

// SessionPath returns the filesystem path for a session file.
func (s *Store) SessionPath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".jsonl")
}

// SubagentPath returns the filesystem path for a subagent session file.
func (s *Store) SubagentPath(parentSessionID, agentSessionID string) string {
	return filepath.Join(s.baseDir, parentSessionID, "subagents", agentSessionID+".jsonl")
}

// Load reads a JSONL session file and reconstructs the Session.
func (s *Store) Load(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadWithoutLock(id)
}

// List returns all sessions sorted by update time (newest first),
// reading from the sessions-index.json for fast listing.
func (s *Store) List() ([]*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadIndex()
	if err != nil {
		// Fall back to scanning JSONL files if index is missing/corrupt.
		return s.listByScanning()
	}

	sessions := make([]*SessionMetadata, 0, len(index.Entries))
	for _, e := range index.Entries {
		if e.IsSidechain {
			continue
		}
		sessions = append(sessions, &SessionMetadata{
			ID:           e.SessionID,
			Title:        e.Summary,
			CreatedAt:    e.Created,
			UpdatedAt:    e.Modified,
			Cwd:          s.cwd,
			MessageCount: e.MessageCount,
		})
	}

	slices.SortFunc(sessions, func(a, b *SessionMetadata) int {
		return b.UpdatedAt.Compare(a.UpdatedAt) // newest first
	})

	return sessions, nil
}

// GetLatest returns the most recently updated session in this project.
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

// Delete removes a session JSONL file and updates the index.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.baseDir, id+".jsonl")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Update index: remove this entry.
	if index, err := s.loadIndex(); err == nil {
		filtered := make([]IndexEntry, 0, len(index.Entries))
		for _, e := range index.Entries {
			if e.SessionID != id {
				filtered = append(filtered, e)
			}
		}
		index.Entries = filtered
		s.saveIndex(index)
	}

	return nil
}

// Cleanup removes sessions older than SessionRetentionDays.
func (s *Store) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -SessionRetentionDays)
	var removed []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		sess, err := s.loadWithoutLock(id)
		if err != nil {
			continue
		}

		if sess.Metadata.UpdatedAt.Before(cutoff) {
			filePath := filepath.Join(s.baseDir, entry.Name())
			_ = os.Remove(filePath)
			removed = append(removed, id)
		}
	}

	// Update index: remove cleaned-up entries.
	if len(removed) > 0 {
		if index, err := s.loadIndex(); err == nil {
			removedSet := make(map[string]bool, len(removed))
			for _, id := range removed {
				removedSet[id] = true
			}
			filtered := make([]IndexEntry, 0, len(index.Entries))
			for _, e := range index.Entries {
				if !removedSet[e.SessionID] {
					filtered = append(filtered, e)
				}
			}
			index.Entries = filtered
			s.saveIndex(index)
		}
	}

	return nil
}

// --- Internal helpers ---

// loadWithoutLock reads a JSONL file and reconstructs a Session (caller must hold lock).
func (s *Store) loadWithoutLock(id string) (*Session, error) {
	return s.loadFromFile(filepath.Join(s.baseDir, id+".jsonl"), id)
}

// Fork creates a new session by copying all entries from the source session.
// The new session gets a fresh ID and timestamps, preserving the full conversation history.
// If source is non-nil, it is used directly (avoiding a redundant disk load).
func (s *Store) Fork(sourceID string, source ...*Session) (*Session, error) {
	// Fetch git branch outside the lock (spawns subprocess).
	gitBranch := getGitBranch(s.cwd)
	newID := generateSessionID()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Use provided session or load from disk.
	var src *Session
	if len(source) > 0 && source[0] != nil {
		src = source[0]
	} else {
		var err error
		src, err = s.loadWithoutLock(sourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to load source session: %w", err)
		}
	}

	forked := &Session{
		Metadata: SessionMetadata{
			ID:              newID,
			Title:           src.Metadata.Title,
			Provider:        src.Metadata.Provider,
			Model:           src.Metadata.Model,
			Cwd:             src.Metadata.Cwd,
			CreatedAt:       now,
			UpdatedAt:       now,
			MessageCount:    len(src.Entries),
			ParentSessionID: sourceID,
		},
		Entries: src.Entries, // share slice; entries are written to file then discarded
		Tasks:   src.Tasks,
	}

	// Write the forked session file.
	filePath := filepath.Join(s.baseDir, newID+".jsonl")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to create forked session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	for _, entry := range src.Entries {
		entry.SessionID = newID
		entry.Timestamp = now
		if entry.GitBranch == "" {
			entry.GitBranch = gitBranch
		}
		if err := enc.Encode(entry); err != nil {
			return nil, fmt.Errorf("failed to write entry: %w", err)
		}
	}

	// Write metadata entry.
	metaEntry := Entry{
		Type:      EntryMetadata,
		SessionID: newID,
		Timestamp: now,
		Metadata: &EntryMetadata_{
			Title:           forked.Metadata.Title,
			Provider:        forked.Metadata.Provider,
			Model:           forked.Metadata.Model,
			Cwd:             forked.Metadata.Cwd,
			CreatedAt:       forked.Metadata.CreatedAt,
			UpdatedAt:       forked.Metadata.UpdatedAt,
			MessageCount:    forked.Metadata.MessageCount,
			ParentSessionID: sourceID,
			Tasks:           forked.Tasks,
		},
	}
	if err := enc.Encode(metaEntry); err != nil {
		return nil, fmt.Errorf("failed to write metadata entry: %w", err)
	}

	// Update the sessions index.
	s.upsertIndexEntry(forked, filePath, false)

	// Copy session memory if it exists.
	srcMemory := filepath.Join(s.baseDir, sourceID, "session-memory", "summary.md")
	if data, err := os.ReadFile(srcMemory); err == nil && len(data) > 0 {
		dstDir := filepath.Join(s.baseDir, newID, "session-memory")
		if err := os.MkdirAll(dstDir, 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(dstDir, "summary.md"), data, 0o644)
		}
	}

	return forked, nil
}

// listByScanning falls back to scanning JSONL files when the index is unavailable.
func (s *Store) listByScanning() ([]*SessionMetadata, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SessionMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read project directory: %w", err)
	}

	sessions := make([]*SessionMetadata, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		sess, err := s.loadWithoutLock(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, &sess.Metadata)
	}

	slices.SortFunc(sessions, func(a, b *SessionMetadata) int {
		return b.UpdatedAt.Compare(a.UpdatedAt) // newest first
	})

	return sessions, nil
}

// encodePath converts a filesystem path to a safe directory name by replacing / with -.
func encodePath(path string) string {
	// Normalize: trim trailing slash
	path = strings.TrimRight(path, "/")
	// Replace / with -
	return strings.ReplaceAll(path, "/", "-")
}

// loadIndex reads the sessions-index.json file.
func (s *Store) loadIndex() (*SessionIndex, error) {
	indexPath := filepath.Join(s.baseDir, indexFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// saveIndex writes the sessions-index.json file.
func (s *Store) saveIndex(index *SessionIndex) {
	indexPath := filepath.Join(s.baseDir, indexFileName)
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(indexPath, data, 0o644)
}

// updateIndexEntry upserts the index entry for the given session.
func (s *Store) updateIndexEntry(session *Session, filePath string) {
	s.upsertIndexEntry(session, filePath, false)
}

// getGitBranch returns the current git branch for the given directory.
// Returns empty string if not a git repo or on error.
func getGitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// generateSessionID generates a UUID v4 session ID.
func generateSessionID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// GenerateShortID generates a short random hex ID for entry UUIDs.
func GenerateShortID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

// LoadSubagent loads a subagent session by its ID, searching across all parent sessions.
// Subagent files are stored at {baseDir}/{parentSessionID}/subagents/{agentID}.jsonl.
func (s *Store) LoadSubagent(agentID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Scan all directories for a subagents/ subdirectory containing this agent ID
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(s.baseDir, entry.Name(), "subagents", agentID+".jsonl")
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		return s.loadFromFile(candidate, agentID)
	}

	return nil, fmt.Errorf("subagent session not found: %s", agentID)
}

// loadFromFile reads a JSONL file at the given path and reconstructs a Session.
// It is the single canonical loader used by loadWithoutLock and LoadSubagent.
func (s *Store) loadFromFile(filePath, id string) (*Session, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()

	var (
		entries []Entry
		meta    *EntryMetadata_
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 16*1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}
		switch entry.Type {
		case EntryUser, EntryAssistant:
			entries = append(entries, entry)
		case EntryMetadata:
			meta = entry.Metadata // last one wins
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// Restore full tool outputs that were truncated and persisted to disk.
	s.restoreToolResults(id, entries)

	sess := &Session{
		Metadata: SessionMetadata{ID: id},
		Entries:  entries,
	}
	applyMetadata(&sess.Metadata, &sess.Tasks, meta)
	return sess, nil
}

// applyMetadata copies fields from the persisted EntryMetadata_ into a SessionMetadata.
// meta may be nil (no metadata entry found), in which case this is a no-op.
func applyMetadata(m *SessionMetadata, tasks *[]tool.TodoTask, meta *EntryMetadata_) {
	if meta == nil {
		return
	}
	m.Title = meta.Title
	m.Provider = meta.Provider
	m.Model = meta.Model
	m.Cwd = meta.Cwd
	m.CreatedAt = meta.CreatedAt
	m.UpdatedAt = meta.UpdatedAt
	m.MessageCount = meta.MessageCount
	m.ParentSessionID = meta.ParentSessionID
	*tasks = meta.Tasks
}

// PersistToolResult saves a large tool result to disk under {sessionID}/tool-results/.
// This prevents oversized tool outputs from bloating the session JSONL file.
func (s *Store) PersistToolResult(sessionID, toolCallID, content string) error {
	dir := filepath.Join(s.baseDir, sessionID, "tool-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create tool-results directory: %w", err)
	}
	filePath := filepath.Join(dir, toolCallID)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write tool result: %w", err)
	}
	return nil
}

// restoreToolResults scans entries for truncated tool outputs and restores the full content
// from the on-disk tool-results directory if available.
// The truncation marker is: "\n\n[Full output persisted to tool-results/{toolUseID}]"
func (s *Store) restoreToolResults(sessionID string, entries []Entry) {
	const marker = "\n\n[Full output persisted to tool-results/"
	toolResultsDir := filepath.Join(s.baseDir, sessionID, "tool-results")

	for i := range entries {
		entry := &entries[i]
		if entry.Message == nil {
			continue
		}
		for j := range entry.Message.Content {
			block := &entry.Message.Content[j]
			if block.Type != "tool_result" {
				continue
			}
			for k := range block.Content {
				sub := &block.Content[k]
				if sub.Type != "text" {
					continue
				}
				idx := strings.Index(sub.Text, marker)
				if idx < 0 {
					continue
				}
				// Extract the tool call ID from the marker suffix
				suffix := sub.Text[idx+len(marker):]
				end := strings.Index(suffix, "]")
				if end < 0 {
					continue
				}
				toolCallID := suffix[:end]
				fullPath := filepath.Join(toolResultsDir, toolCallID)
				if data, err := os.ReadFile(fullPath); err == nil {
					sub.Text = string(data)
				}
			}
		}
	}
}

// SaveSessionMemory persists a compaction summary to {sessionID}/session-memory/summary.md.
func (s *Store) SaveSessionMemory(sessionID, summary string) error {
	dir := filepath.Join(s.baseDir, sessionID, "session-memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create session-memory directory: %w", err)
	}
	filePath := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(filePath, []byte(summary), 0o644); err != nil {
		return fmt.Errorf("failed to write session memory: %w", err)
	}
	return nil
}

// LoadSessionMemory reads the persisted compaction summary for a session.
// Returns empty string and nil error if the file does not exist.
func (s *Store) LoadSessionMemory(sessionID string) (string, error) {
	filePath := filepath.Join(s.baseDir, sessionID, "session-memory", "summary.md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read session memory: %w", err)
	}
	return string(data), nil
}

// updateIndexEntrySidechain upserts a sidechain (subagent) entry in the index.
func (s *Store) updateIndexEntrySidechain(session *Session, filePath string) {
	s.upsertIndexEntry(session, filePath, true)
}

// upsertIndexEntry is the shared implementation for updating index entries.
// When isSidechain is true, the entry is marked as a sidechain and GitBranch is omitted.
func (s *Store) upsertIndexEntry(session *Session, filePath string, isSidechain bool) {
	index, err := s.loadIndex()
	if err != nil {
		index = &SessionIndex{
			Version:      1,
			OriginalPath: s.cwd,
		}
	}

	firstPrompt := ExtractFirstUserText(session.Entries)
	if len(firstPrompt) > 200 {
		firstPrompt = firstPrompt[:200]
	}

	var mtime int64
	if info, err := os.Stat(filePath); err == nil {
		mtime = info.ModTime().UnixMilli()
	}

	newEntry := IndexEntry{
		SessionID:    session.Metadata.ID,
		FullPath:     filePath,
		FileMtime:    mtime,
		FirstPrompt:  firstPrompt,
		Summary:      session.Metadata.Title,
		MessageCount: session.Metadata.MessageCount,
		Created:      session.Metadata.CreatedAt,
		Modified:     session.Metadata.UpdatedAt,
		IsSidechain:  isSidechain,
	}
	if !isSidechain {
		newEntry.GitBranch = getGitBranch(s.cwd)
	}

	// Upsert: replace existing or append.
	found := false
	for i, e := range index.Entries {
		if e.SessionID == session.Metadata.ID {
			index.Entries[i] = newEntry
			found = true
			break
		}
	}
	if !found {
		index.Entries = append(index.Entries, newEntry)
	}

	s.saveIndex(index)
}

package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/transcriptstore"
)

const SessionRetentionDays = 30

type Store struct {
	mu              sync.RWMutex
	cwd             string
	projectID       string
	projectDir      string
	transcriptStore *transcriptstore.FileStore
}

type Snapshot struct {
	Metadata SessionMetadata
	Entries  []Entry
	Tasks    []tool.TodoTask
}

type Session = Snapshot

func NewStore(cwd string) (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	projectID := encodePath(cwd)
	projectDir := filepath.Join(homeDir, ".gen", "projects", projectID)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	txStore, err := transcriptstore.NewFileStore(projectDir, projectID)
	if err != nil {
		return nil, err
	}

	return &Store{
		cwd:             cwd,
		projectID:       projectID,
		projectDir:      projectDir,
		transcriptStore: txStore,
	}, nil
}

func NewStoreWithDir(dir string) *Store {
	_ = os.MkdirAll(dir, 0o755)
	txStore, _ := transcriptstore.NewFileStore(dir, encodePath(dir))
	return &Store{
		cwd:             dir,
		projectID:       encodePath(dir),
		projectDir:      dir,
		transcriptStore: txStore,
	}
}

func (s *Store) SessionPath(sessionID string) string {
	if s.transcriptStore != nil {
		return s.transcriptStore.TranscriptPath(sessionID)
	}
	return filepath.Join(s.projectDir, "transcripts", sessionID+".jsonl")
}

func (s *Store) List() ([]*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items, err := s.transcriptStore.List(context.Background(), s.projectID, transcriptstore.ListOptions{})
	if err != nil {
		return nil, err
	}

	out := make([]*SessionMetadata, 0, len(items))
	for _, item := range items {
		meta := transcriptstore.MetadataFromListItem(item, s.cwd)
		out = append(out, &meta)
	}
	return out, nil
}

func (s *Store) GetLatest() (*Snapshot, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return s.Load(items[0].ID)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.transcriptStore.Delete(context.Background(), id); err != nil {
		return err
	}
	_ = os.RemoveAll(s.toolResultsDir(id))
	return nil
}

func (s *Store) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.transcriptStore.List(context.Background(), s.projectID, transcriptstore.ListOptions{IncludeSidechain: true})
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -SessionRetentionDays)
	for _, item := range items {
		if item.UpdatedAt.Before(cutoff) {
			_ = s.transcriptStore.Delete(context.Background(), item.TranscriptID)
			_ = os.RemoveAll(s.toolResultsDir(item.TranscriptID))
		}
	}
	return nil
}

func (s *Store) Load(id string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess := s.loadSnapshot(context.Background(), id)
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return sess, nil
}

func (s *Store) Save(sess *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transcriptStore == nil {
		return fmt.Errorf("transcript store not configured")
	}
	if sess == nil {
		return fmt.Errorf("session is nil")
	}

	now := time.Now()
	NormalizeMetadata(&sess.Metadata, sess.Entries, s.cwd, now)

	gitBranch := getGitBranch(s.cwd)
	nodes := EntriesToNodes(sess.Entries, sess.Metadata.ID, sess.Metadata.Cwd, sess.Metadata.CreatedAt, gitBranch)

	return s.transcriptStore.Replace(context.Background(), transcriptstore.ReplaceCommand{
		Transcript: TranscriptFromSnapshot(sess, nodes, sess.Tasks),
	})
}

func (s *Store) Fork(sourceID string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newID := generateSessionID()
	if err := s.transcriptStore.Fork(context.Background(), transcriptstore.ForkCommand{
		SourceTranscriptID: sourceID,
		NewTranscriptID:    newID,
		Time:               time.Now(),
	}); err != nil {
		return nil, err
	}
	forked := s.loadSnapshot(context.Background(), newID)
	if forked == nil {
		return nil, fmt.Errorf("failed to load forked session: %s", newID)
	}
	return forked, nil
}

func (s *Store) PersistToolResult(sessionID, toolCallID, content string) error {
	dir := s.toolResultsDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create tool result dir: %w", err)
	}
	filePath := filepath.Join(dir, toolCallID)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write tool result: %w", err)
	}
	return nil
}

func (s *Store) SaveSubagentConversation(parentSessionID, title, modelID, cwd string, messages []message.Message) (string, string, error) {
	entries := MessagesToEntries(messages)
	if len(entries) == 0 {
		return "", "", nil
	}
	if title == "" {
		title = "Subagent"
	}
	if cwd == "" {
		cwd = s.cwd
	}

	sess := &Snapshot{
		Metadata: SessionMetadata{
			Title:           title,
			Model:           modelID,
			Cwd:             cwd,
			ParentSessionID: parentSessionID,
		},
		Entries: entries,
	}
	if err := s.Save(sess); err != nil {
		return "", "", err
	}
	return sess.Metadata.ID, s.SessionPath(sess.Metadata.ID), nil
}

func (s *Store) LoadSubagentMessages(agentID string) ([]message.Message, error) {
	sess, err := s.Load(agentID)
	if err != nil {
		return nil, err
	}
	msgs := EntriesToMessages(sess.Entries)
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages found in session %s", agentID)
	}
	return msgs, nil
}

func (s *Store) SaveSessionMemory(sessionID, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.transcriptStore.PatchState(context.Background(), transcriptstore.PatchStateCommand{
		TranscriptID: sessionID,
		Time:         time.Now(),
		Ops:          []transcriptstore.PatchOp{transcriptstore.PatchSummary(summary)},
	})
}

func (s *Store) LoadSessionMemory(sessionID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	transcript, err := s.transcriptStore.Load(context.Background(), sessionID)
	if err != nil {
		return "", nil
	}
	return transcript.State.Summary, nil
}

func (s *Store) toolResultsDir(sessionID string) string {
	return filepath.Join(s.projectDir, "blobs", "tool-result", sessionID)
}

func (s *Store) loadSnapshot(ctx context.Context, sessionID string) *Snapshot {
	if s.transcriptStore == nil || sessionID == "" {
		return nil
	}
	transcript, err := s.transcriptStore.Load(ctx, sessionID)
	if err != nil || transcript == nil {
		return nil
	}
	transcriptstore.HydrateToolResultNodes(transcript.ID, transcript.Messages, func(toolCallID string) (string, error) {
		data, err := os.ReadFile(filepath.Join(s.toolResultsDir(transcript.ID), toolCallID))
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	sess := &Snapshot{
		Metadata: transcriptstore.MetadataFromTranscript(transcript),
		Entries:  EntriesFromNodes(transcript.ID, transcript.Messages),
		Tasks:    transcriptstore.TodoTasksFromView(transcript.State.Tasks),
	}

	if sess.Metadata.Title == "" {
		sess.Metadata.Title = GenerateTitle(sess.Entries)
	}
	if sess.Metadata.LastPrompt == "" {
		sess.Metadata.LastPrompt = ExtractLastUserText(sess.Entries)
	}
	return sess
}

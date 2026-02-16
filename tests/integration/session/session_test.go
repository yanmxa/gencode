package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/session"
)

// newTestStore creates a Store using a temp directory instead of ~/.gen/sessions/.
func newTestStore(t *testing.T) *session.Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return session.NewStoreWithDir(dir)
}

// writeSessionFile writes a session JSON file directly to disk,
// bypassing Save() which overrides UpdatedAt.
func writeSessionFile(t *testing.T, dir string, sess *session.Session) {
	t.Helper()
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, sess.Metadata.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestSession_SaveAndLoad(t *testing.T) {
	store := newTestStore(t)

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       "test-1",
			Title:    "Test Session",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
		},
		Messages: []session.StoredMessage{
			{Role: string(message.RoleUser), Content: "hello"},
			{Role: string(message.RoleAssistant), Content: "hi there"},
		},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load("test-1")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Metadata.Title != "Test Session" {
		t.Errorf("expected title 'Test Session', got %q", loaded.Metadata.Title)
	}
	if loaded.Metadata.Provider != "fake" {
		t.Errorf("expected provider 'fake', got %q", loaded.Metadata.Provider)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Errorf("expected first message 'hello', got %q", loaded.Messages[0].Content)
	}
}

func TestSession_List(t *testing.T) {
	store := newTestStore(t)

	for i, title := range []string{"First", "Second", "Third"} {
		sess := &session.Session{
			Metadata: session.SessionMetadata{
				ID:        title,
				Title:     title,
				UpdatedAt: time.Now().Add(time.Duration(i) * time.Second),
			},
		}
		if err := store.Save(sess); err != nil {
			t.Fatalf("Save(%s) error: %v", title, err)
		}
		// Small sleep so timestamps differ
		time.Sleep(10 * time.Millisecond)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(list))
	}

	// Sorted by update time, newest first
	if list[0].Title != "Third" {
		t.Errorf("expected newest first ('Third'), got %q", list[0].Title)
	}
}

func TestSession_GetLatest(t *testing.T) {
	store := newTestStore(t)

	sess1 := &session.Session{
		Metadata: session.SessionMetadata{ID: "old", Title: "Old"},
	}
	if err := store.Save(sess1); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	sess2 := &session.Session{
		Metadata: session.SessionMetadata{ID: "new", Title: "New"},
	}
	if err := store.Save(sess2); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	latest, err := store.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest() error: %v", err)
	}

	if latest.Metadata.Title != "New" {
		t.Errorf("expected latest 'New', got %q", latest.Metadata.Title)
	}
}

func TestSession_GetLatestByCwd(t *testing.T) {
	store := newTestStore(t)

	sess1 := &session.Session{
		Metadata: session.SessionMetadata{ID: "proj-a", Title: "Project A", Cwd: "/a"},
	}
	sess2 := &session.Session{
		Metadata: session.SessionMetadata{ID: "proj-b", Title: "Project B", Cwd: "/b"},
	}
	if err := store.Save(sess1); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if err := store.Save(sess2); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.GetLatestByCwd("/b")
	if err != nil {
		t.Fatalf("GetLatestByCwd() error: %v", err)
	}
	if loaded.Metadata.Title != "Project B" {
		t.Errorf("expected 'Project B', got %q", loaded.Metadata.Title)
	}

	_, err = store.GetLatestByCwd("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cwd")
	}
}

func TestSession_Delete(t *testing.T) {
	store := newTestStore(t)

	sess := &session.Session{
		Metadata: session.SessionMetadata{ID: "to-delete", Title: "Delete Me"},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if err := store.Delete("to-delete"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := store.Load("to-delete")
	if err == nil {
		t.Error("expected error loading deleted session")
	}
}

func TestSession_Cleanup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := session.NewStoreWithDir(dir)

	// Write old session file directly (bypass Save which overrides UpdatedAt)
	oldTime := time.Now().AddDate(0, 0, -(session.SessionRetentionDays + 1))
	oldSess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:        "old-session",
			Title:     "Old",
			CreatedAt: oldTime,
			UpdatedAt: oldTime,
		},
	}
	writeSessionFile(t, dir, oldSess)

	// Save a recent session normally
	newSess := &session.Session{
		Metadata: session.SessionMetadata{ID: "new-session", Title: "New"},
	}
	if err := store.Save(newSess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if err := store.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Old should be gone
	_, err := store.Load("old-session")
	if err == nil {
		t.Error("expected old session to be cleaned up")
	}

	// New should remain
	_, err = store.Load("new-session")
	if err != nil {
		t.Errorf("new session should still exist: %v", err)
	}
}

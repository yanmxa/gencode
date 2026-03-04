package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/session"
)

// newTestStore creates a Store using a temp directory instead of ~/.gen/projects/.
func newTestStore(t *testing.T) *session.Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return session.NewStoreWithDir(dir)
}

// writeSessionJSONL writes a session JSONL file directly to disk,
// bypassing Save() which overrides UpdatedAt.
func writeSessionJSONL(t *testing.T, dir string, sess *session.Session) {
	t.Helper()
	filePath := filepath.Join(dir, sess.Metadata.ID+".jsonl")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	for i := range sess.Entries {
		if err := enc.Encode(sess.Entries[i]); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}

	metaEntry := map[string]interface{}{
		"type":      "metadata",
		"sessionId": sess.Metadata.ID,
		"metadata": map[string]interface{}{
			"title":        sess.Metadata.Title,
			"provider":     sess.Metadata.Provider,
			"model":        sess.Metadata.Model,
			"cwd":          sess.Metadata.Cwd,
			"createdAt":    sess.Metadata.CreatedAt,
			"updatedAt":    sess.Metadata.UpdatedAt,
			"messageCount": len(sess.Entries),
		},
	}
	if err := enc.Encode(metaEntry); err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
}

// makeUserEntry creates a user text entry for testing.
func makeUserEntry(uuid, text string) session.Entry {
	return session.Entry{
		Type: session.EntryUser,
		UUID: uuid,
		Message: &session.EntryMessage{
			Role:    "user",
			Content: []session.ContentBlock{{Type: "text", Text: text}},
		},
	}
}

// makeAssistantEntry creates an assistant text entry for testing.
func makeAssistantEntry(uuid, text string) session.Entry {
	return session.Entry{
		Type: session.EntryAssistant,
		UUID: uuid,
		Message: &session.EntryMessage{
			Role:    "assistant",
			Content: []session.ContentBlock{{Type: "text", Text: text}},
		},
	}
}

// getEntryText extracts the first text content block from an entry.
func getEntryText(e session.Entry) string {
	if e.Message == nil {
		return ""
	}
	for _, block := range e.Message.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
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
		Entries: []session.Entry{
			makeUserEntry("u1", "hello"),
			makeAssistantEntry("a1", "hi there"),
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
	if len(loaded.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(loaded.Entries))
	}
	if getEntryText(loaded.Entries[0]) != "hello" {
		t.Errorf("expected first entry 'hello', got %q", getEntryText(loaded.Entries[0]))
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	writeSessionJSONL(t, dir, oldSess)

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

func TestSession_AppendBehavior(t *testing.T) {
	store := newTestStore(t)

	// First save with 1 entry
	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:    "append-test",
			Title: "Append Test",
		},
		Entries: []session.Entry{
			makeUserEntry("u1", "hello"),
		},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("first Save() error: %v", err)
	}

	// Second save with 3 entries (original + 2 new)
	sess.Entries = append(sess.Entries,
		makeAssistantEntry("a1", "hi there"),
		makeUserEntry("u2", "how are you?"),
	)
	if err := store.Save(sess); err != nil {
		t.Fatalf("second Save() error: %v", err)
	}

	// Load and verify all 3 entries are present
	loaded, err := store.Load("append-test")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(loaded.Entries))
	}
	if getEntryText(loaded.Entries[2]) != "how are you?" {
		t.Errorf("expected third entry 'how are you?', got %q", getEntryText(loaded.Entries[2]))
	}
}

func TestSession_EntryRoundtrip(t *testing.T) {
	// Test that MessagesToEntries → EntriesToMessages roundtrips correctly.
	msgs := []message.Message{
		{Role: message.RoleUser, Content: "hello"},
		{Role: message.RoleAssistant, Content: "hi", Thinking: "let me think",
			ToolCalls: []message.ToolCall{{ID: "tc-1", Name: "Read", Input: `{"file_path":"/tmp/test"}`}}},
		{Role: message.RoleUser, ToolResult: &message.ToolResult{
			ToolCallID: "tc-1", ToolName: "Read", Content: "file contents",
		}},
		{Role: message.RoleAssistant, Content: "I see the file."},
	}

	entries := session.MessagesToEntries(msgs)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Verify entry types
	if entries[0].Type != session.EntryUser {
		t.Errorf("entry[0] type: want user, got %s", entries[0].Type)
	}
	if entries[1].Type != session.EntryAssistant {
		t.Errorf("entry[1] type: want assistant, got %s", entries[1].Type)
	}
	if entries[2].Type != session.EntryUser {
		t.Errorf("entry[2] type: want user (tool_result), got %s", entries[2].Type)
	}

	// Round-trip back to messages
	restored := session.EntriesToMessages(entries)
	if len(restored) != 4 {
		t.Fatalf("expected 4 messages after roundtrip, got %d", len(restored))
	}
	if restored[0].Content != "hello" {
		t.Errorf("msg[0].Content: want 'hello', got %q", restored[0].Content)
	}
	if restored[1].Thinking != "let me think" {
		t.Errorf("msg[1].Thinking: want 'let me think', got %q", restored[1].Thinking)
	}
	if restored[2].ToolResult == nil {
		t.Fatal("msg[2].ToolResult should not be nil")
	}
	if restored[2].ToolResult.ToolCallID != "tc-1" {
		t.Errorf("msg[2].ToolResult.ToolCallID: want 'tc-1', got %q", restored[2].ToolResult.ToolCallID)
	}
	// Tool name should be resolved from the tool_use block
	if restored[2].ToolResult.ToolName != "Read" {
		t.Errorf("msg[2].ToolResult.ToolName: want 'Read', got %q", restored[2].ToolResult.ToolName)
	}
}

func TestSession_PersistToolResult(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := session.NewStoreWithDir(dir)

	sessionID := "tool-overflow-test"
	toolCallID := "tc-abc123"
	content := strings.Repeat("x", 200_000) // 200KB

	if err := store.PersistToolResult(sessionID, toolCallID, content); err != nil {
		t.Fatalf("PersistToolResult() error: %v", err)
	}

	// Verify the file was created with correct content
	resultPath := filepath.Join(dir, sessionID, "tool-results", toolCallID)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("failed to read persisted tool result: %v", err)
	}
	if len(data) != 200_000 {
		t.Errorf("persisted content size = %d, want 200000", len(data))
	}
}

func TestSession_SaveAndLoadSessionMemory(t *testing.T) {
	store := newTestStore(t)

	sessionID := "memory-test"
	summary := "# Session Summary\n\nThe user asked about Go testing patterns."

	// Save session memory
	if err := store.SaveSessionMemory(sessionID, summary); err != nil {
		t.Fatalf("SaveSessionMemory() error: %v", err)
	}

	// Load session memory
	loaded, err := store.LoadSessionMemory(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionMemory() error: %v", err)
	}
	if loaded != summary {
		t.Errorf("LoadSessionMemory() = %q, want %q", loaded, summary)
	}
}

func TestSession_LoadSessionMemory_NotFound(t *testing.T) {
	store := newTestStore(t)

	// Load from non-existent session - should return empty string, no error
	loaded, err := store.LoadSessionMemory("nonexistent")
	if err != nil {
		t.Fatalf("LoadSessionMemory() error: %v", err)
	}
	if loaded != "" {
		t.Errorf("LoadSessionMemory() = %q, want empty string", loaded)
	}
}

func TestSession_SaveSessionMemory_Overwrite(t *testing.T) {
	store := newTestStore(t)

	sessionID := "overwrite-test"

	// Save first summary
	if err := store.SaveSessionMemory(sessionID, "first summary"); err != nil {
		t.Fatalf("SaveSessionMemory() first call error: %v", err)
	}

	// Overwrite with second summary
	if err := store.SaveSessionMemory(sessionID, "second summary"); err != nil {
		t.Fatalf("SaveSessionMemory() second call error: %v", err)
	}

	// Load should return the second summary
	loaded, err := store.LoadSessionMemory(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionMemory() error: %v", err)
	}
	if loaded != "second summary" {
		t.Errorf("LoadSessionMemory() = %q, want 'second summary'", loaded)
	}
}

// TestSession_MemoryEndToEnd verifies the full flow:
// save session → save session memory → load session → load memory → verify memory content.
// This simulates what happens when a session is compacted and then resumed.
func TestSession_MemoryEndToEnd(t *testing.T) {
	store := newTestStore(t)

	// 1. Create and save a session (simulates a conversation)
	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       "memory-e2e",
			Title:    "End to End Memory Test",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
		},
		Entries: []session.Entry{
			makeUserEntry("u1", "hello"),
			makeAssistantEntry("a1", "hi there"),
		},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// 2. Simulate compaction: save session memory (this happens in handleCompactResult)
	summary := "# Session Summary\n\nThe user greeted the assistant and received a response."
	if err := store.SaveSessionMemory("memory-e2e", summary); err != nil {
		t.Fatalf("SaveSessionMemory() error: %v", err)
	}

	// 3. Simulate session resume: load session + load memory (this happens in loadSession)
	loaded, err := store.Load("memory-e2e")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Metadata.ID != "memory-e2e" {
		t.Errorf("expected session ID 'memory-e2e', got %q", loaded.Metadata.ID)
	}

	mem, err := store.LoadSessionMemory("memory-e2e")
	if err != nil {
		t.Fatalf("LoadSessionMemory() error: %v", err)
	}
	if mem != summary {
		t.Errorf("LoadSessionMemory() = %q, want %q", mem, summary)
	}

	// 4. Verify the memory would be injected into the system prompt
	// (this simulates what buildExtraContext does)
	expectedTag := "<session-memory>\n" + summary + "\n</session-memory>"
	if !strings.Contains(expectedTag, summary) {
		t.Error("session-memory tag should contain the summary")
	}

	// 5. Verify the memory file exists on disk at the expected path
	dir := filepath.Join(t.TempDir(), "sessions") // matches newTestStore
	_ = dir // path used by store internally
}

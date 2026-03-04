package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/message"
)

func TestGetLatest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir, cwd: tmpDir, projectDir: tmpDir}

	sessA := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-a",
			Cwd:       tmpDir,
			CreatedAt: time.Now().Add(-3 * time.Minute),
			UpdatedAt: time.Now().Add(-3 * time.Minute),
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "a1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hello from alpha"}}},
		}},
	}
	sessB := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-b",
			Cwd:       tmpDir,
			CreatedAt: time.Now().Add(-2 * time.Minute),
			UpdatedAt: time.Now().Add(-2 * time.Minute),
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "b1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hello from beta"}}},
		}},
	}
	sessA2 := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-a2",
			Cwd:       tmpDir,
			CreatedAt: time.Now().Add(-1 * time.Minute),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "a21",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "second session"}}},
		}},
	}

	for _, s := range []*Session{sessA, sessB, sessA2} {
		if err := store.Save(s); err != nil {
			t.Fatalf("failed to save session %s: %v", s.Metadata.ID, err)
		}
	}

	// GetLatest should return sess-a2 (most recent overall by UpdatedAt)
	result, err := store.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if result.Metadata.ID != "sess-a2" {
		t.Errorf("expected sess-a2, got %s", result.Metadata.ID)
	}
}

func TestStoreCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-cleanup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir, cwd: tmpDir, projectDir: tmpDir}

	// Save a new session normally
	newSess := &Session{
		Metadata: SessionMetadata{
			ID:  "new-sess",
			Cwd: tmpDir,
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "n1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "new"}}},
		}},
	}
	if err := store.Save(newSess); err != nil {
		t.Fatal(err)
	}

	// Write an old session JSONL file directly with an expired timestamp
	oldTime := time.Now().AddDate(0, 0, -(SessionRetentionDays + 1))
	oldSess := &Session{
		Metadata: SessionMetadata{
			ID:        "old-sess",
			Cwd:       tmpDir,
			CreatedAt: oldTime,
			UpdatedAt: oldTime,
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "o1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "old"}}},
		}},
	}
	writeSessionJSONL(t, tmpDir, oldSess)

	// Run cleanup
	if err := store.Cleanup(); err != nil {
		t.Fatal(err)
	}

	// Old session should be deleted
	_, err = os.Stat(filepath.Join(tmpDir, "old-sess.jsonl"))
	if !os.IsNotExist(err) {
		t.Error("expected old session to be cleaned up")
	}

	// New session should still exist
	_, err = os.Stat(filepath.Join(tmpDir, "new-sess.jsonl"))
	if err != nil {
		t.Error("expected new session to still exist")
	}
}

func TestAppendBehavior(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-append-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir, cwd: tmpDir, projectDir: tmpDir}

	// First save with 1 entry
	sess := &Session{
		Metadata: SessionMetadata{
			ID:    "append-test",
			Title: "Append Test",
			Cwd:   tmpDir,
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "u1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hello"}}},
		}},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("first Save() error: %v", err)
	}

	// Second save with 3 entries (original + 2 new)
	sess.Entries = append(sess.Entries,
		Entry{
			Type: EntryAssistant, UUID: "a1",
			Message: &EntryMessage{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "hi there"}}},
		},
		Entry{
			Type: EntryUser, UUID: "u2",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "how are you?"}}},
		},
	)
	if err := store.Save(sess); err != nil {
		t.Fatalf("second Save() error: %v", err)
	}

	// Load and verify all 3 entries
	loaded, err := store.Load("append-test")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(loaded.Entries))
	}
	if getEntryText(loaded.Entries[0]) != "hello" {
		t.Errorf("expected first entry 'hello', got %q", getEntryText(loaded.Entries[0]))
	}
	if getEntryText(loaded.Entries[1]) != "hi there" {
		t.Errorf("expected second entry 'hi there', got %q", getEntryText(loaded.Entries[1]))
	}
	if getEntryText(loaded.Entries[2]) != "how are you?" {
		t.Errorf("expected third entry 'how are you?', got %q", getEntryText(loaded.Entries[2]))
	}

	// Verify metadata was updated
	if loaded.Metadata.MessageCount != 3 {
		t.Errorf("expected messageCount 3, got %d", loaded.Metadata.MessageCount)
	}
}

func TestSaveSubagent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-subagent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir, cwd: tmpDir, projectDir: tmpDir}

	// Save a parent session
	parent := &Session{
		Metadata: SessionMetadata{
			ID:  "parent-sess",
			Cwd: tmpDir,
		},
		Entries: []Entry{{
			Type: EntryUser, UUID: "p1",
			Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "run a task"}}},
		}},
	}
	if err := store.Save(parent); err != nil {
		t.Fatalf("Save parent: %v", err)
	}

	// Save a subagent session
	subagent := &Session{
		Metadata: SessionMetadata{
			Title: "Explore codebase",
			Cwd:   tmpDir,
		},
		Entries: []Entry{
			{
				Type: EntryUser, UUID: "s1",
				Message: &EntryMessage{Role: "user", Content: []ContentBlock{{Type: "text", Text: "find all Go files"}}},
			},
			{
				Type: EntryAssistant, UUID: "s2",
				Message: &EntryMessage{Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "Found 42 Go files."}}},
			},
		},
	}
	if err := store.SaveSubagent("parent-sess", subagent); err != nil {
		t.Fatalf("SaveSubagent: %v", err)
	}

	// Verify the JSONL file was created under parent-sess/subagents/
	subagentsDir := filepath.Join(tmpDir, "parent-sess", "subagents")
	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		t.Fatalf("ReadDir subagents: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 subagent file, got %d", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".jsonl" {
		t.Errorf("expected .jsonl extension, got %s", entries[0].Name())
	}

	// Verify subagent metadata
	if subagent.Metadata.ParentSessionID != "parent-sess" {
		t.Errorf("expected parentSessionId 'parent-sess', got %q", subagent.Metadata.ParentSessionID)
	}
	if subagent.Metadata.MessageCount != 2 {
		t.Errorf("expected messageCount 2, got %d", subagent.Metadata.MessageCount)
	}

	// Verify List() filters out sidechain sessions
	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range sessions {
		if s.ID == subagent.Metadata.ID {
			t.Error("List() should not include sidechain sessions")
		}
	}
	// Should still include the parent
	found := false
	for _, s := range sessions {
		if s.ID == "parent-sess" {
			found = true
			break
		}
	}
	if !found {
		t.Error("List() should include the parent session")
	}

	// Verify index has isSidechain=true for the subagent
	index, err := store.loadIndex()
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	for _, e := range index.Entries {
		if e.SessionID == subagent.Metadata.ID {
			if !e.IsSidechain {
				t.Error("expected isSidechain=true for subagent index entry")
			}
		}
		if e.SessionID == "parent-sess" {
			if e.IsSidechain {
				t.Error("expected isSidechain=false for parent index entry")
			}
		}
	}
}

func TestMessagesToEntries(t *testing.T) {
	msgs := []message.Message{
		{Role: message.RoleUser, Content: "hello"},
		{Role: message.RoleAssistant, Content: "hi", Thinking: "let me think"},
		{Role: message.RoleUser, ToolResult: &message.ToolResult{
			ToolCallID: "tc-1",
			ToolName:   "Read",
			Content:    "file contents",
		}},
	}

	entries := MessagesToEntries(msgs)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// First entry: user text
	if entries[0].Type != EntryUser {
		t.Errorf("expected type 'user', got %q", entries[0].Type)
	}
	if getEntryText(entries[0]) != "hello" {
		t.Errorf("expected text 'hello', got %q", getEntryText(entries[0]))
	}

	// Second entry: assistant with thinking
	if entries[1].Type != EntryAssistant {
		t.Errorf("expected type 'assistant', got %q", entries[1].Type)
	}
	thinkingFound := false
	for _, block := range entries[1].Message.Content {
		if block.Type == "thinking" && block.Thinking == "let me think" {
			thinkingFound = true
		}
	}
	if !thinkingFound {
		t.Error("expected thinking content preserved")
	}

	// Third entry: tool_result
	if entries[2].Type != EntryUser {
		t.Errorf("expected type 'user' for tool result, got %q", entries[2].Type)
	}
	toolResultFound := false
	for _, block := range entries[2].Message.Content {
		if block.Type == "tool_result" && block.ToolUseID == "tc-1" {
			toolResultFound = true
		}
	}
	if !toolResultFound {
		t.Error("expected tool_result content block")
	}

	// Verify parentUuid chain
	if entries[0].ParentUuid != nil {
		t.Error("expected first entry to have nil parentUuid")
	}
	if entries[1].ParentUuid == nil || *entries[1].ParentUuid != entries[0].UUID {
		t.Error("expected second entry parentUuid to point to first entry")
	}
	if entries[2].ParentUuid == nil || *entries[2].ParentUuid != entries[1].UUID {
		t.Error("expected third entry parentUuid to point to second entry")
	}
}

// writeSessionJSONL writes a session JSONL file directly to disk,
// bypassing Save() which overrides UpdatedAt.
func writeSessionJSONL(t *testing.T, dir string, sess *Session) {
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

	metaEntry := Entry{
		Type:      EntryMetadata,
		SessionID: sess.Metadata.ID,
		Metadata: &EntryMetadata_{
			Title:        sess.Metadata.Title,
			Cwd:          sess.Metadata.Cwd,
			CreatedAt:    sess.Metadata.CreatedAt,
			UpdatedAt:    sess.Metadata.UpdatedAt,
			MessageCount: len(sess.Entries),
		},
	}
	if err := enc.Encode(metaEntry); err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
}

// getEntryText extracts the first text content from an entry's content blocks.
func getEntryText(entry Entry) string {
	if entry.Message == nil {
		return ""
	}
	for _, block := range entry.Message.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

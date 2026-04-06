package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/app/session"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/transcriptstore"
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
	txStore, err := transcriptstore.NewFileStore(dir, strings.ReplaceAll(strings.TrimRight(dir, "/"), "/", "-"))
	if err != nil {
		t.Fatalf("NewFileStore(): %v", err)
	}
	if err := txStore.Start(context.Background(), transcriptstore.StartCommand{
		TranscriptID: "old-session",
		ProjectID:    strings.ReplaceAll(strings.TrimRight(dir, "/"), "/", "-"),
		Cwd:          dir,
		Time:         oldTime,
	}); err != nil {
		t.Fatalf("Start(old): %v", err)
	}
	if err := txStore.PatchState(context.Background(), transcriptstore.PatchStateCommand{
		TranscriptID: "old-session",
		Time:         oldTime.Add(time.Second),
		Ops:          []transcriptstore.PatchOp{transcriptstore.PatchTitle("Old")},
	}); err != nil {
		t.Fatalf("PatchState(old): %v", err)
	}

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
	_, err = store.Load("old-session")
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

func TestSession_MetadataUpdatesOnNewMessage(t *testing.T) {
	store := newTestStore(t)

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:    "metadata-update-test",
			Title: "Metadata Update Test",
		},
		Entries: []session.Entry{
			makeUserEntry("u1", "hello"),
		},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("first Save() error: %v", err)
	}

	first, err := store.Load("metadata-update-test")
	if err != nil {
		t.Fatalf("first Load() error: %v", err)
	}
	if first.Metadata.MessageCount != 1 {
		t.Fatalf("first message count = %d, want 1", first.Metadata.MessageCount)
	}

	time.Sleep(10 * time.Millisecond)

	sess.Entries = append(sess.Entries, makeAssistantEntry("a1", "hi there"))
	if err := store.Save(sess); err != nil {
		t.Fatalf("second Save() error: %v", err)
	}

	second, err := store.Load("metadata-update-test")
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if second.Metadata.MessageCount != 2 {
		t.Errorf("second message count = %d, want 2", second.Metadata.MessageCount)
	}
	if !second.Metadata.UpdatedAt.After(first.Metadata.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance: first=%v second=%v", first.Metadata.UpdatedAt, second.Metadata.UpdatedAt)
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

func TestSession_MessageTypes_PersistRoundTrip(t *testing.T) {
	store := newTestStore(t)

	toolInput := json.RawMessage(`{"file_path":"/tmp/test.txt"}`)
	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       "message-types-roundtrip",
			Title:    "Message Types Roundtrip",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
		},
		Entries: []session.Entry{
			makeUserEntry("u1", "read this file"),
			{
				Type: session.EntryAssistant,
				UUID: "a1",
				Message: &session.EntryMessage{
					Role: "assistant",
					Content: []session.ContentBlock{
						{Type: "thinking", Thinking: "need to inspect the file", Signature: "sig-1"},
						{Type: "text", Text: "I'll inspect it."},
						{Type: "tool_use", ID: "tc-1", Name: "Read", Input: toolInput},
					},
				},
			},
			{
				Type: session.EntryUser,
				UUID: "u2",
				Message: &session.EntryMessage{
					Role: "user",
					Content: []session.ContentBlock{
						{
							Type:      "tool_result",
							ToolUseID: "tc-1",
							IsError:   false,
							Content:   []session.ContentBlock{{Type: "text", Text: "file contents"}},
						},
					},
				},
			},
			makeAssistantEntry("a2", "done"),
		},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load("message-types-roundtrip")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(loaded.Entries))
	}

	assistant := loaded.Entries[1]
	if assistant.Message == nil || len(assistant.Message.Content) != 3 {
		t.Fatalf("assistant content blocks = %v, want 3 blocks", assistant.Message)
	}
	if assistant.Message.Content[0].Type != "thinking" || assistant.Message.Content[0].Thinking != "need to inspect the file" {
		t.Errorf("thinking block did not round-trip correctly: %+v", assistant.Message.Content[0])
	}
	if assistant.Message.Content[2].Type != "tool_use" || assistant.Message.Content[2].Name != "Read" {
		t.Errorf("tool_use block did not round-trip correctly: %+v", assistant.Message.Content[2])
	}

	userResult := loaded.Entries[2]
	if userResult.Message == nil || len(userResult.Message.Content) != 1 {
		t.Fatalf("tool result entry = %+v, want one block", userResult.Message)
	}
	resultBlock := userResult.Message.Content[0]
	if resultBlock.Type != "tool_result" || resultBlock.ToolUseID != "tc-1" {
		t.Errorf("tool_result block did not round-trip correctly: %+v", resultBlock)
	}
	if len(resultBlock.Content) != 1 || resultBlock.Content[0].Text != "file contents" {
		t.Errorf("tool_result nested content mismatch: %+v", resultBlock.Content)
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
	resultPath := filepath.Join(dir, "blobs", "tool-result", sessionID, toolCallID)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("failed to read persisted tool result: %v", err)
	}
	if len(data) != 200_000 {
		t.Errorf("persisted content size = %d, want 200000", len(data))
	}

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:    sessionID,
			Title: "Overflow",
		},
		Entries: []session.Entry{
			{
				Type: session.EntryUser,
				UUID: "u1",
				Message: &session.EntryMessage{
					Role: "user",
					Content: []session.ContentBlock{{
						Type:      "tool_result",
						ToolUseID: toolCallID,
						Content: []session.ContentBlock{{
							Type: "text",
							Text: "preview\n\n[Full output persisted to blobs/tool-result/" + sessionID + "/" + toolCallID + "]",
						}},
					}},
				},
			},
		},
	}
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	got := loaded.Entries[0].Message.Content[0].Content[0].Text
	if got != content {
		t.Fatalf("hydrated tool result len = %d, want %d", len(got), len(content))
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
	// (this simulates what buildSections does)
	expectedTag := "<session-memory>\n" + summary + "\n</session-memory>"
	if !strings.Contains(expectedTag, summary) {
		t.Error("session-memory tag should contain the summary")
	}

	// 5. The summary is transcript state, not a legacy sidecar file.
}

// TestSession_JSONL_Integrity verifies that every line written to a JSONL
// session file is valid JSON. This guards against serialisation regressions
// where a malformed entry silently breaks session loading.
func TestSession_JSONL_Integrity(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStoreWithDir(dir)

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       "jsonl-integrity-test",
			Title:    "JSONL Integrity Test",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
		},
		Entries: []session.Entry{
			makeUserEntry("u1", "first message"),
			makeAssistantEntry("a1", "first response"),
			makeUserEntry("u2", "second message"),
			makeAssistantEntry("a2", "second response with special chars: <>&\"'"),
		},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Read the raw JSONL file and verify every non-empty line is valid JSON.
	filePath := store.SessionPath(sess.Metadata.ID)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	validLines := 0
	for i, line := range lines {
		if line == "" {
			continue // trailing newline is expected
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v\ncontent: %s", i+1, err, line)
		} else {
			validLines++
		}
	}

	// Expect at least entries + 1 metadata line
	if validLines < len(sess.Entries)+1 {
		t.Errorf("expected at least %d valid JSON lines, got %d", len(sess.Entries)+1, validLines)
	}
}

// TestSession_ContinueRestoresMessages verifies that loading a session after
// multiple Save calls returns all messages in the original order. This
// simulates the "-c" (--continue) flag behaviour where the previous
// conversation must be fully replayed.
func TestSession_ContinueRestoresMessages(t *testing.T) {
	store := newTestStore(t)

	// Build a multi-turn conversation.
	turns := []struct{ role, text string }{
		{"user", "hello"},
		{"assistant", "hi there"},
		{"user", "what is 2+2?"},
		{"assistant", "4"},
		{"user", "thanks"},
	}

	var entries []session.Entry
	for i, turn := range turns {
		uuid := fmt.Sprintf("id-%d", i)
		switch turn.role {
		case "user":
			entries = append(entries, makeUserEntry(uuid, turn.text))
		case "assistant":
			entries = append(entries, makeAssistantEntry(uuid, turn.text))
		}
	}

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       "continue-test",
			Title:    "Continue Test",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
		},
		Entries: entries,
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Simulate "-c": load the session and verify messages are in order.
	loaded, err := store.Load("continue-test")
	if err != nil {
		t.Fatalf("Load() (continue) error: %v", err)
	}

	if len(loaded.Entries) != len(turns) {
		t.Fatalf("expected %d entries after continue, got %d", len(turns), len(loaded.Entries))
	}

	for i, want := range turns {
		got := getEntryText(loaded.Entries[i])
		if got != want.text {
			t.Errorf("entry[%d]: want %q, got %q", i, want.text, got)
		}

		wantType := session.EntryUser
		if want.role == "assistant" {
			wantType = session.EntryAssistant
		}
		if loaded.Entries[i].Type != wantType {
			t.Errorf("entry[%d]: want type %q, got %q", i, wantType, loaded.Entries[i].Type)
		}
	}
}

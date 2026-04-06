package session

import (
	"os"
	"testing"
)

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(): %v", err)
	}

	sess := &Snapshot{
		Metadata: SessionMetadata{
			ID:       "roundtrip",
			Title:    "Round Trip",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
			Summary:  "summary",
		},
		Entries: []Entry{
			{
				Type: EntryUser,
				UUID: "u1",
				Message: &EntryMessage{
					Role:    "user",
					Content: []ContentBlock{{Type: "text", Text: "hello"}},
				},
			},
			{
				Type: EntryAssistant,
				UUID: "a1",
				Message: &EntryMessage{
					Role:    "assistant",
					Content: []ContentBlock{{Type: "text", Text: "hi"}},
				},
			},
		},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	loaded, err := store.Load(sess.Metadata.ID)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if loaded.Metadata.Title != "Round Trip" {
		t.Fatalf("Title = %q, want %q", loaded.Metadata.Title, "Round Trip")
	}
	if loaded.Metadata.Summary != "summary" {
		t.Fatalf("Summary = %q, want %q", loaded.Metadata.Summary, "summary")
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(loaded.Entries))
	}
	if got := loaded.Entries[0].Message.Content[0].Text; got != "hello" {
		t.Fatalf("first entry text = %q, want %q", got, "hello")
	}
}

func TestStoreFork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(): %v", err)
	}

	source := &Snapshot{
		Metadata: SessionMetadata{
			ID:    "source",
			Title: "Source",
			Cwd:   "/tmp/project",
		},
		Entries: []Entry{
			{
				Type: EntryUser,
				UUID: "u1",
				Message: &EntryMessage{
					Role:    "user",
					Content: []ContentBlock{{Type: "text", Text: "hello"}},
				},
			},
		},
	}

	if err := store.Save(source); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	forked, err := store.Fork(source.Metadata.ID)
	if err != nil {
		t.Fatalf("Fork(): %v", err)
	}
	if forked.Metadata.ID == source.Metadata.ID {
		t.Fatal("expected forked session ID to differ from source")
	}
	if forked.Metadata.ParentSessionID != source.Metadata.ID {
		t.Fatalf("ParentSessionID = %q, want %q", forked.Metadata.ParentSessionID, source.Metadata.ID)
	}
	if _, err := os.Stat(store.SessionPath(forked.Metadata.ID)); err != nil {
		t.Fatalf("fork transcript missing: %v", err)
	}
}

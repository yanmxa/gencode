package sessionui

import (
	"os"
	"testing"

	"github.com/yanmxa/gencode/internal/session"
)

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(): %v", err)
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:       "roundtrip",
			Title:    "Round Trip",
			Provider: "fake",
			Model:    "fake-model",
			Cwd:      "/tmp/project",
			Summary:  "summary",
		},
		Entries: []session.Entry{
			{
				Type: session.EntryUser,
				UUID: "u1",
				Message: &session.EntryMessage{
					Role:    "user",
					Content: []session.ContentBlock{{Type: "text", Text: "hello"}},
				},
			},
			{
				Type: session.EntryAssistant,
				UUID: "a1",
				Message: &session.EntryMessage{
					Role:    "assistant",
					Content: []session.ContentBlock{{Type: "text", Text: "hi"}},
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

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore(): %v", err)
	}

	source := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:    "source",
			Title: "Source",
			Cwd:   "/tmp/project",
		},
		Entries: []session.Entry{
			{
				Type: session.EntryUser,
				UUID: "u1",
				Message: &session.EntryMessage{
					Role:    "user",
					Content: []session.ContentBlock{{Type: "text", Text: "hello"}},
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

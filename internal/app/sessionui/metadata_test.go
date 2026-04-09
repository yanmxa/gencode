package sessionui

import (
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tracker"
	"github.com/yanmxa/gencode/internal/transcript"
)

func TestNormalizeMetadataAppliesDefaults(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	meta := session.SessionMetadata{}
	entries := []session.Entry{{
		Message: &session.EntryMessage{
			Role:    "user",
			Content: []session.ContentBlock{{Type: "text", Text: "continue"}},
		},
	}}

	session.NormalizeMetadata(&meta, entries, "/tmp/project", now)
	if meta.ID == "" {
		t.Fatal("expected generated metadata ID")
	}
	if meta.CreatedAt != now || meta.UpdatedAt != now {
		t.Fatalf("unexpected timestamps: %+v", meta)
	}
	if meta.Cwd != "/tmp/project" || meta.MessageCount != 1 {
		t.Fatalf("unexpected defaults: %+v", meta)
	}
	if meta.LastPrompt != "continue" {
		t.Fatalf("LastPrompt = %q, want %q", meta.LastPrompt, "continue")
	}
	if meta.Title == "" {
		t.Fatal("expected generated title")
	}
}

func TestTranscriptFromSnapshotProjectsMetadataAndTasks(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:              "session-1",
			ParentSessionID: "parent-1",
			Cwd:             "/tmp/project",
			CreatedAt:       now,
			UpdatedAt:       now.Add(time.Minute),
			Provider:        "openai",
			Model:           "gpt-test",
			Title:           "Title",
			LastPrompt:      "continue",
			Summary:         "summary",
			Tag:             "tag",
			Mode:            "plan",
		},
	}
	nodes := []transcript.Node{{ID: "m1", Role: "user"}}
	tasks := []tracker.Task{{ID: "1", Subject: "Refactor", Status: tracker.StatusPending}}

	tr := session.TranscriptFromSnapshot(sess, nodes, tasks)
	if tr.ID != "session-1" || tr.ParentID != "parent-1" {
		t.Fatalf("unexpected transcript identity: %+v", tr)
	}
	if tr.Cwd != "/tmp/project" || tr.Provider != "openai" || tr.Model != "gpt-test" {
		t.Fatalf("unexpected transcript metadata: %+v", tr)
	}
	if len(tr.Messages) != 1 || tr.Messages[0].ID != "m1" {
		t.Fatalf("unexpected transcript messages: %+v", tr.Messages)
	}
	if tr.State.Title != "Title" || tr.State.LastPrompt != "continue" || tr.State.Summary != "summary" {
		t.Fatalf("unexpected transcript state: %+v", tr.State)
	}
	if len(tr.State.Tasks) != 1 || tr.State.Tasks[0].Subject != "Refactor" {
		t.Fatalf("unexpected transcript tasks: %+v", tr.State.Tasks)
	}
}

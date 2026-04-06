package session

import (
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/transcriptstore"
)

func TestNormalizeMetadataAppliesDefaults(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	meta := SessionMetadata{}
	entries := []Entry{{
		Message: &EntryMessage{
			Role:    "user",
			Content: []ContentBlock{{Type: "text", Text: "continue"}},
		},
	}}

	NormalizeMetadata(&meta, entries, "/tmp/project", now)
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
	sess := &Snapshot{
		Metadata: SessionMetadata{
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
	nodes := []transcriptstore.Node{{ID: "m1", Role: "user"}}
	tasks := []tool.TodoTask{{ID: "1", Subject: "Refactor", Status: tool.TodoStatusPending}}

	transcript := TranscriptFromSnapshot(sess, nodes, tasks)
	if transcript.ID != "session-1" || transcript.ParentID != "parent-1" {
		t.Fatalf("unexpected transcript identity: %+v", transcript)
	}
	if transcript.Cwd != "/tmp/project" || transcript.Provider != "openai" || transcript.Model != "gpt-test" {
		t.Fatalf("unexpected transcript metadata: %+v", transcript)
	}
	if len(transcript.Messages) != 1 || transcript.Messages[0].ID != "m1" {
		t.Fatalf("unexpected transcript messages: %+v", transcript.Messages)
	}
	if transcript.State.Title != "Title" || transcript.State.LastPrompt != "continue" || transcript.State.Summary != "summary" {
		t.Fatalf("unexpected transcript state: %+v", transcript.State)
	}
	if len(transcript.State.Tasks) != 1 || transcript.State.Tasks[0].Subject != "Refactor" {
		t.Fatalf("unexpected transcript tasks: %+v", transcript.State.Tasks)
	}
}

package session

import (
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/transcriptstore"
)

func TestEntriesToNodesAppliesDefaults(t *testing.T) {
	createdAt := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	entries := []Entry{
		{
			Message: &EntryMessage{
				Role:    "user",
				Content: []ContentBlock{{Type: "text", Text: "hello"}},
			},
		},
		{
			IsSidechain: true,
			AgentID:     "agent-1",
			Message: &EntryMessage{
				Role:    "assistant",
				Content: []ContentBlock{{Type: "text", Text: "hi"}},
			},
		},
	}

	nodes := EntriesToNodes(entries, "session-1", "/tmp/project", createdAt, "main")
	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}
	if nodes[0].Role != "user" || nodes[1].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", nodes)
	}
	if nodes[0].Cwd != "/tmp/project" || nodes[1].Cwd != "/tmp/project" {
		t.Fatalf("cwd defaults not applied: %#v", nodes)
	}
	if nodes[0].GitBranch != "main" || nodes[1].GitBranch != "main" {
		t.Fatalf("git branch defaults not applied: %#v", nodes)
	}
	if nodes[1].ParentID != nodes[0].ID {
		t.Fatalf("ParentID = %q, want %q", nodes[1].ParentID, nodes[0].ID)
	}
	if entries[0].SessionID != "session-1" || entries[1].SessionID != "session-1" {
		t.Fatalf("session IDs not backfilled on entries: %#v", entries)
	}
	if entries[0].Type != EntryUser || entries[1].Type != EntryAssistant {
		t.Fatalf("entry types not inferred: %#v", entries)
	}
	if entries[0].Timestamp.IsZero() || entries[1].Timestamp.IsZero() {
		t.Fatalf("timestamps not backfilled: %#v", entries)
	}
	if !entries[1].Timestamp.After(entries[0].Timestamp) {
		t.Fatalf("timestamps not ordered: %#v", entries)
	}
}

func TestEntriesFromNodesRoundTrip(t *testing.T) {
	parentID := "msg-1"
	at := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	entries := EntriesFromNodes("session-1", []transcriptstore.Node{
		{
			ID:        parentID,
			Role:      "user",
			Time:      at,
			Cwd:       "/tmp/project",
			GitBranch: "main",
			Content:   []ContentBlock{{Type: "text", Text: "hello"}},
		},
		{
			ID:          "msg-2",
			ParentID:    parentID,
			Role:        "assistant",
			Time:        at.Add(time.Second),
			Cwd:         "/tmp/project",
			GitBranch:   "main",
			AgentID:     "agent-1",
			IsSidechain: true,
			Content:     []ContentBlock{{Type: "text", Text: "hi"}},
		},
	})

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].SessionID != "session-1" || entries[1].SessionID != "session-1" {
		t.Fatalf("session IDs missing: %#v", entries)
	}
	if entries[0].ParentUuid != nil {
		t.Fatalf("expected first entry parent to be nil, got %v", *entries[0].ParentUuid)
	}
	if entries[1].ParentUuid == nil || *entries[1].ParentUuid != parentID {
		t.Fatalf("unexpected second parent: %#v", entries[1].ParentUuid)
	}
	if entries[1].Type != EntryAssistant || !entries[1].IsSidechain || entries[1].AgentID != "agent-1" {
		t.Fatalf("node fields not projected: %#v", entries[1])
	}
}

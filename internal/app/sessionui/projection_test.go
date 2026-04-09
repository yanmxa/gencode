package sessionui

import (
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/transcript"
)

func TestEntriesToNodesAppliesDefaults(t *testing.T) {
	createdAt := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	entries := []session.Entry{
		{
			Message: &session.EntryMessage{
				Role:    "user",
				Content: []session.ContentBlock{{Type: "text", Text: "hello"}},
			},
		},
		{
			IsSidechain: true,
			AgentID:     "agent-1",
			Message: &session.EntryMessage{
				Role:    "assistant",
				Content: []session.ContentBlock{{Type: "text", Text: "hi"}},
			},
		},
	}

	nodes := session.EntriesToNodes(entries, "session-1", "/tmp/project", createdAt, "main")
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
	if entries[0].Type != session.EntryUser || entries[1].Type != session.EntryAssistant {
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
	entries := session.EntriesFromNodes("session-1", []transcript.Node{
		{
			ID:        parentID,
			Role:      "user",
			Time:      at,
			Cwd:       "/tmp/project",
			GitBranch: "main",
			Content:   []session.ContentBlock{{Type: "text", Text: "hello"}},
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
			Content:     []session.ContentBlock{{Type: "text", Text: "hi"}},
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
	if entries[1].Type != session.EntryAssistant || !entries[1].IsSidechain || entries[1].AgentID != "agent-1" {
		t.Fatalf("node fields not projected: %#v", entries[1])
	}
}

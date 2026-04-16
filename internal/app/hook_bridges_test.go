package app

import (
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestBuildTaskNotificationIncludesResumableAgentIdentity(t *testing.T) {
	tracker.DefaultStore.Reset()
	t.Cleanup(tracker.DefaultStore.Reset)

	item, ok := buildTaskNotification(task.TaskInfo{
		ID:             "task-123",
		Type:           task.TaskTypeAgent,
		Description:    "Inspect code",
		Status:         task.StatusCompleted,
		Output:         "summary",
		OutputFile:     "/tmp/transcripts/agent-session-123.jsonl",
		AgentType:      "Explore",
		AgentName:      "Audit Worker",
		AgentSessionID: "agent-session-123",
		TurnCount:      4,
		TokenUsage:     120,
	})
	if !ok {
		t.Fatal("expected notification")
	}

	for _, want := range []string{
		"<agent-type>Explore</agent-type>",
		"<agent>Audit Worker</agent>",
		"<agent-id>agent-session-123</agent-id>",
		"<output-file>/tmp/transcripts/agent-session-123.jsonl</output-file>",
		"<turns>4</turns>",
		"<total_tokens>120</total_tokens>",
	} {
		if !strings.Contains(item.ContinuationPrompt, want) {
			t.Fatalf("ContinuationPrompt missing %q:\n%s", want, item.ContinuationPrompt)
		}
	}
}

func TestBuildTaskNotificationIncludesBatchContext(t *testing.T) {
	tracker.DefaultStore.Reset()
	t.Cleanup(tracker.DefaultStore.Reset)

	batch := tracker.DefaultStore.Create("2 background agents launched", "", "", map[string]any{
		backgroundTrackerKindKey:   backgroundTrackerKindBatch,
		backgroundTrackerCompleted: 1,
		backgroundTrackerTotal:     2,
		backgroundTrackerFailures:  0,
	})
	_ = tracker.DefaultStore.Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child1 := tracker.DefaultStore.Create("dir-audit: Directory structure audit", "", "", map[string]any{
		backgroundTrackerKindKey:      backgroundTrackerKindWorker,
		backgroundTrackerParentID:     batch.ID,
		backgroundTrackerTaskID:       "task-123",
		backgroundTrackerAgentType:    "Explore",
		backgroundTrackerStatusDetail: string(task.StatusCompleted),
	})
	_ = tracker.DefaultStore.Update(child1.ID, tracker.WithStatus(tracker.StatusCompleted))
	child2 := tracker.DefaultStore.Create("naming-audit: Package naming audit", "", "", map[string]any{
		backgroundTrackerKindKey:      backgroundTrackerKindWorker,
		backgroundTrackerParentID:     batch.ID,
		backgroundTrackerTaskID:       "task-456",
		backgroundTrackerAgentType:    "Plan",
		backgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child2.ID, tracker.WithStatus(tracker.StatusInProgress))

	item, ok := buildTaskNotification(task.TaskInfo{
		ID:             "task-123",
		Type:           task.TaskTypeAgent,
		Description:    "Inspect code",
		Status:         task.StatusCompleted,
		AgentType:      "Explore",
		AgentName:      "Audit Worker",
		AgentSessionID: "agent-session-123",
	})
	if !ok {
		t.Fatal("expected notification")
	}

	for _, want := range []string{
		"<batch>",
		"<id>" + batch.ID + "</id>",
		"<summary>2 background agents launched is 1/2 complete</summary>",
		`<task task-id="task-123" status="completed" agent-type="Explore" current="true">dir-audit: Directory structure audit</task>`,
		`<task task-id="task-456" status="running" agent-type="Plan">naming-audit: Package naming audit</task>`,
	} {
		if !strings.Contains(item.ContinuationPrompt, want) {
			t.Fatalf("ContinuationPrompt missing %q:\n%s", want, item.ContinuationPrompt)
		}
	}
	if item.Batch == nil || item.Batch.ID != batch.ID {
		t.Fatalf("expected batch snapshot for %s, got %#v", batch.ID, item.Batch)
	}
}

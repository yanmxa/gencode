package app

import (
	"strings"
	"testing"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestBuildTaskNotificationIncludesResumableAgentIdentity(t *testing.T) {
	tracker.DefaultStore.Reset()
	t.Cleanup(tracker.DefaultStore.Reset)

	info := task.TaskInfo{
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
	}
	item, ok := appagent.BuildTaskNotification(appagent.TaskNotificationInput{
		Info:    info,
		Subject: taskSubject(info),
		Batch:   appagent.SnapshotBackgroundBatchForTask(info.ID),
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
		appagent.BackgroundTrackerKindKey:   appagent.BackgroundTrackerKindBatch,
		appagent.BackgroundTrackerCompleted: 1,
		appagent.BackgroundTrackerTotal:     2,
		appagent.BackgroundTrackerFailures:  0,
	})
	_ = tracker.DefaultStore.Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child1 := tracker.DefaultStore.Create("dir-audit: Directory structure audit", "", "", map[string]any{
		appagent.BackgroundTrackerKindKey:      appagent.BackgroundTrackerKindWorker,
		appagent.BackgroundTrackerParentID:     batch.ID,
		appagent.BackgroundTrackerTaskID:       "task-123",
		appagent.BackgroundTrackerAgentType:    "Explore",
		appagent.BackgroundTrackerStatusDetail: string(task.StatusCompleted),
	})
	_ = tracker.DefaultStore.Update(child1.ID, tracker.WithStatus(tracker.StatusCompleted))
	child2 := tracker.DefaultStore.Create("naming-audit: Package naming audit", "", "", map[string]any{
		appagent.BackgroundTrackerKindKey:      appagent.BackgroundTrackerKindWorker,
		appagent.BackgroundTrackerParentID:     batch.ID,
		appagent.BackgroundTrackerTaskID:       "task-456",
		appagent.BackgroundTrackerAgentType:    "Plan",
		appagent.BackgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child2.ID, tracker.WithStatus(tracker.StatusInProgress))

	info2 := task.TaskInfo{
		ID:             "task-123",
		Type:           task.TaskTypeAgent,
		Description:    "Inspect code",
		Status:         task.StatusCompleted,
		AgentType:      "Explore",
		AgentName:      "Audit Worker",
		AgentSessionID: "agent-session-123",
	}
	item, ok := appagent.BuildTaskNotification(appagent.TaskNotificationInput{
		Info:    info2,
		Subject: taskSubject(info2),
		Batch:   appagent.SnapshotBackgroundBatchForTask(info2.ID),
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

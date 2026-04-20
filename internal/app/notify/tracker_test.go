package notify

import (
	"testing"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestTrackWorkerCreatesEntry(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	bt := NewBackgroundTracker(tracker.Default())
	bt.TrackWorker(BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "dir-audit",
		AgentType:   "Explore",
		Description: "Directory structure audit",
	})

	tasks := tracker.Default().List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 tracker task, got %d", len(tasks))
	}
	if tasks[0].Status != tracker.StatusInProgress {
		t.Fatalf("status = %q, want %q", tasks[0].Status, tracker.StatusInProgress)
	}
	if metadataString(tasks[0].Metadata, metaTaskID) != "bg-1" {
		t.Fatalf("task ID metadata = %q", metadataString(tasks[0].Metadata, metaTaskID))
	}
}

func TestCompleteWorkerUpdatesStatus(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	bt := NewBackgroundTracker(tracker.Default())
	bt.TrackWorker(BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "dir-audit",
		AgentType:   "Explore",
		Description: "Directory audit",
	})

	bt.CompleteWorker(task.TaskInfo{
		ID:     "bg-1",
		Type:   task.TaskTypeAgent,
		Status: task.StatusCompleted,
	})

	tasks := tracker.Default().List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 tracker task, got %d", len(tasks))
	}
	if tasks[0].Status != tracker.StatusCompleted {
		t.Fatalf("status = %q, want %q", tasks[0].Status, tracker.StatusCompleted)
	}
	if metadataString(tasks[0].Metadata, metaStatusDetail) != string(task.StatusCompleted) {
		t.Fatalf("status detail = %q", metadataString(tasks[0].Metadata, metaStatusDetail))
	}
}

func TestCompleteWorkerTracksFailure(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	bt := NewBackgroundTracker(tracker.Default())
	bt.TrackWorker(BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "fix-auth",
		AgentType:   "general-purpose",
		Description: "Fix auth module",
	})

	bt.CompleteWorker(task.TaskInfo{
		ID:     "bg-1",
		Type:   task.TaskTypeAgent,
		Status: task.StatusFailed,
		Error:  "compilation error",
	})

	tasks := tracker.Default().List()
	if metadataString(tasks[0].Metadata, metaStatusDetail) != string(task.StatusFailed) {
		t.Fatalf("status detail = %q, want %q", metadataString(tasks[0].Metadata, metaStatusDetail), task.StatusFailed)
	}
}

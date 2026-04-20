package hub

import (
	"testing"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func metadataStr(metadata map[string]any, key string) string {
	if v, ok := metadata[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func TestTrackWorkerCreatesEntry(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	TrackWorker(tracker.Default(), BackgroundTaskLaunch{
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
	if metadataStr(tasks[0].Metadata, metaTaskID) != "bg-1" {
		t.Fatalf("task ID metadata = %q", metadataStr(tasks[0].Metadata, metaTaskID))
	}
}

func TestCompleteWorkerUpdatesStatus(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	TrackWorker(tracker.Default(), BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "dir-audit",
		AgentType:   "Explore",
		Description: "Directory audit",
	})

	CompleteWorker(tracker.Default(), task.TaskInfo{
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
	if metadataStr(tasks[0].Metadata, metaStatusDetail) != string(task.StatusCompleted) {
		t.Fatalf("status detail = %q", metadataStr(tasks[0].Metadata, metaStatusDetail))
	}
}

func TestCompleteWorkerTracksFailure(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	TrackWorker(tracker.Default(), BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "fix-auth",
		AgentType:   "general-purpose",
		Description: "Fix auth module",
	})

	CompleteWorker(tracker.Default(), task.TaskInfo{
		ID:     "bg-1",
		Type:   task.TaskTypeAgent,
		Status: task.StatusFailed,
		Error:  "compilation error",
	})

	tasks := tracker.Default().List()
	if metadataStr(tasks[0].Metadata, metaStatusDetail) != string(task.StatusFailed) {
		t.Fatalf("status detail = %q, want %q", metadataStr(tasks[0].Metadata, metaStatusDetail), task.StatusFailed)
	}
}

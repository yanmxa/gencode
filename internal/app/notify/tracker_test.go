package notify

import (
	"testing"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestBackgroundTaskTrackerCreatesBatchAndChildren(t *testing.T) {
	tracker.DefaultStore.Reset()
	orchestration.DefaultStore.Reset()
	t.Cleanup(func() {
		tracker.DefaultStore.Reset()
		orchestration.DefaultStore.Reset()
	})

	batchKey := "call-1,call-2"
	parentID := EnsureBackgroundBatchTracker(batchKey, 2)
	if parentID == "" {
		t.Fatal("expected batch tracker to be created")
	}

	launch1 := BackgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "dir-audit",
		AgentType:   "Explore",
		Description: "Directory structure audit",
	}
	childID1 := EnsureBackgroundWorkerTracker(launch1, parentID, batchKey)
	if childID1 == "" {
		t.Fatal("expected worker tracker for bg-1")
	}
	RecordBackgroundTaskLaunch(launch1, parentID, batchKey, 2)

	launch2 := BackgroundTaskLaunch{
		TaskID:      "bg-2",
		AgentName:   "naming-audit",
		AgentType:   "Plan",
		Description: "Package naming audit",
		ResumeID:    "agent-2",
	}
	childID2 := EnsureBackgroundWorkerTracker(launch2, parentID, batchKey)
	if childID2 == "" {
		t.Fatal("expected worker tracker for bg-2")
	}
	reconcileBackgroundBatch(parentID)
	RecordBackgroundTaskLaunch(launch2, parentID, batchKey, 2)

	tasks := tracker.DefaultStore.List()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tracker tasks, got %d", len(tasks))
	}

	batch := findTrackerByMetadata(BackgroundTrackerKindKey, BackgroundTrackerKindBatch)
	if batch == nil {
		t.Fatal("expected batch tracker")
	}
	if batch.Status != tracker.StatusInProgress {
		t.Fatalf("batch status = %q, want %q", batch.Status, tracker.StatusInProgress)
	}

	child := findTrackerByMetadata(BackgroundTrackerTaskID, "bg-2")
	if child == nil {
		t.Fatal("expected child tracker for bg-2")
	}
	if MetadataString(child.Metadata, BackgroundTrackerParentID) != batch.ID {
		t.Fatalf("child parent id = %q, want %q", MetadataString(child.Metadata, BackgroundTrackerParentID), batch.ID)
	}
	if MetadataString(child.Metadata, BackgroundTrackerAgentID) != "agent-2" {
		t.Fatalf("child agent id = %q", MetadataString(child.Metadata, BackgroundTrackerAgentID))
	}

	batchSnapshot, ok := orchestration.DefaultStore.SnapshotBatchForTask("bg-2")
	if !ok {
		t.Fatal("expected orchestration batch snapshot")
	}
	if batchSnapshot.Total != 2 {
		t.Fatalf("batch total = %d, want 2", batchSnapshot.Total)
	}
	if len(batchSnapshot.Workers) != 2 {
		t.Fatalf("workers = %d, want 2", len(batchSnapshot.Workers))
	}
}

func TestUpdateBackgroundWorkerTrackerReconcilesBatch(t *testing.T) {
	tracker.DefaultStore.Reset()
	orchestration.DefaultStore.Reset()
	t.Cleanup(func() {
		tracker.DefaultStore.Reset()
		orchestration.DefaultStore.Reset()
	})

	batch := tracker.DefaultStore.Create("2 background agents launched", "", "", map[string]any{
		BackgroundTrackerKindKey:   BackgroundTrackerKindBatch,
		BackgroundTrackerBatchKey:  "call-1,call-2",
		BackgroundTrackerCompleted: 0,
		BackgroundTrackerTotal:     2,
		BackgroundTrackerFailures:  0,
	})
	_ = tracker.DefaultStore.Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child1 := tracker.DefaultStore.Create("dir-audit: Directory audit", "", "", map[string]any{
		BackgroundTrackerKindKey:      BackgroundTrackerKindWorker,
		BackgroundTrackerParentID:     batch.ID,
		BackgroundTrackerTaskID:       "bg-1",
		BackgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child1.ID, tracker.WithStatus(tracker.StatusInProgress))
	child2 := tracker.DefaultStore.Create("naming-audit: Naming audit", "", "", map[string]any{
		BackgroundTrackerKindKey:      BackgroundTrackerKindWorker,
		BackgroundTrackerParentID:     batch.ID,
		BackgroundTrackerTaskID:       "bg-2",
		BackgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child2.ID, tracker.WithStatus(tracker.StatusInProgress))

	UpdateBackgroundWorkerTracker(task.TaskInfo{ID: "bg-1", Type: task.TaskTypeAgent, Status: task.StatusCompleted})
	batchAfterFirst, _ := tracker.DefaultStore.Get(batch.ID)
	if batchAfterFirst.Status != tracker.StatusInProgress {
		t.Fatalf("batch status after first completion = %q", batchAfterFirst.Status)
	}
	if MetadataString(batchAfterFirst.Metadata, BackgroundTrackerCompleted) != "1" {
		t.Fatalf("batch completed count = %q", MetadataString(batchAfterFirst.Metadata, BackgroundTrackerCompleted))
	}

	UpdateBackgroundWorkerTracker(task.TaskInfo{ID: "bg-2", Type: task.TaskTypeAgent, Status: task.StatusFailed, Error: "boom"})
	batchAfterSecond, _ := tracker.DefaultStore.Get(batch.ID)
	if batchAfterSecond.Status != tracker.StatusCompleted {
		t.Fatalf("batch status after second completion = %q", batchAfterSecond.Status)
	}
	if MetadataString(batchAfterSecond.Metadata, BackgroundTrackerFailures) != "1" {
		t.Fatalf("batch failure count = %q", MetadataString(batchAfterSecond.Metadata, BackgroundTrackerFailures))
	}

	snapshot, ok := orchestration.DefaultStore.SnapshotBatchForTask("bg-2")
	if !ok {
		t.Fatal("expected orchestration batch snapshot")
	}
	if snapshot.Completed != 2 {
		t.Fatalf("completed = %d, want 2", snapshot.Completed)
	}
	if snapshot.Failures != 1 {
		t.Fatalf("failures = %d, want 1", snapshot.Failures)
	}
}

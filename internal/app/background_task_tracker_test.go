package app

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

	// Create a batch with 2 workers using the functions called by agent_events.go
	batchKey := "call-1,call-2"
	parentID := ensureBackgroundBatchTracker(batchKey, 2)
	if parentID == "" {
		t.Fatal("expected batch tracker to be created")
	}

	launch1 := backgroundTaskLaunch{
		TaskID:      "bg-1",
		AgentName:   "dir-audit",
		AgentType:   "Explore",
		Description: "Directory structure audit",
	}
	childID1 := ensureBackgroundWorkerTracker(launch1, parentID, batchKey)
	if childID1 == "" {
		t.Fatal("expected worker tracker for bg-1")
	}
	recordBackgroundTaskLaunch(launch1, parentID, batchKey, 2)

	launch2 := backgroundTaskLaunch{
		TaskID:      "bg-2",
		AgentName:   "naming-audit",
		AgentType:   "Plan",
		Description: "Package naming audit",
		ResumeID:    "agent-2",
	}
	childID2 := ensureBackgroundWorkerTracker(launch2, parentID, batchKey)
	if childID2 == "" {
		t.Fatal("expected worker tracker for bg-2")
	}
	reconcileBackgroundBatch(parentID)
	recordBackgroundTaskLaunch(launch2, parentID, batchKey, 2)

	tasks := tracker.DefaultStore.List()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tracker tasks, got %d", len(tasks))
	}

	batch := findTrackerByMetadata(backgroundTrackerKindKey, backgroundTrackerKindBatch)
	if batch == nil {
		t.Fatal("expected batch tracker")
	}
	if batch.Status != tracker.StatusInProgress {
		t.Fatalf("batch status = %q, want %q", batch.Status, tracker.StatusInProgress)
	}

	child := findTrackerByMetadata(backgroundTrackerTaskID, "bg-2")
	if child == nil {
		t.Fatal("expected child tracker for bg-2")
	}
	if metadataString(child.Metadata, backgroundTrackerParentID) != batch.ID {
		t.Fatalf("child parent id = %q, want %q", metadataString(child.Metadata, backgroundTrackerParentID), batch.ID)
	}
	if metadataString(child.Metadata, backgroundTrackerAgentID) != "agent-2" {
		t.Fatalf("child agent id = %q", metadataString(child.Metadata, backgroundTrackerAgentID))
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
		backgroundTrackerKindKey:   backgroundTrackerKindBatch,
		backgroundTrackerBatchKey:  "call-1,call-2",
		backgroundTrackerCompleted: 0,
		backgroundTrackerTotal:     2,
		backgroundTrackerFailures:  0,
	})
	_ = tracker.DefaultStore.Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child1 := tracker.DefaultStore.Create("dir-audit: Directory audit", "", "", map[string]any{
		backgroundTrackerKindKey:      backgroundTrackerKindWorker,
		backgroundTrackerParentID:     batch.ID,
		backgroundTrackerTaskID:       "bg-1",
		backgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child1.ID, tracker.WithStatus(tracker.StatusInProgress))
	child2 := tracker.DefaultStore.Create("naming-audit: Naming audit", "", "", map[string]any{
		backgroundTrackerKindKey:      backgroundTrackerKindWorker,
		backgroundTrackerParentID:     batch.ID,
		backgroundTrackerTaskID:       "bg-2",
		backgroundTrackerStatusDetail: string(task.StatusRunning),
	})
	_ = tracker.DefaultStore.Update(child2.ID, tracker.WithStatus(tracker.StatusInProgress))

	updateBackgroundWorkerTracker(task.TaskInfo{ID: "bg-1", Type: task.TaskTypeAgent, Status: task.StatusCompleted})
	batchAfterFirst, _ := tracker.DefaultStore.Get(batch.ID)
	if batchAfterFirst.Status != tracker.StatusInProgress {
		t.Fatalf("batch status after first completion = %q", batchAfterFirst.Status)
	}
	if metadataString(batchAfterFirst.Metadata, backgroundTrackerCompleted) != "1" {
		t.Fatalf("batch completed count = %q", metadataString(batchAfterFirst.Metadata, backgroundTrackerCompleted))
	}

	updateBackgroundWorkerTracker(task.TaskInfo{ID: "bg-2", Type: task.TaskTypeAgent, Status: task.StatusFailed, Error: "boom"})
	batchAfterSecond, _ := tracker.DefaultStore.Get(batch.ID)
	if batchAfterSecond.Status != tracker.StatusCompleted {
		t.Fatalf("batch status after second completion = %q", batchAfterSecond.Status)
	}
	if metadataString(batchAfterSecond.Metadata, backgroundTrackerFailures) != "1" {
		t.Fatalf("batch failure count = %q", metadataString(batchAfterSecond.Metadata, backgroundTrackerFailures))
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

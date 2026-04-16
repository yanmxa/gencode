package orchestration

import "testing"

func TestSnapshotBuildsDecisionFromBatchState(t *testing.T) {
	store := newStore()
	store.RecordLaunch(Launch{
		TaskID:       "task-1",
		AgentID:      "agent-1",
		AgentType:    "Explore",
		Status:       "completed",
		BatchID:      "batch-1",
		BatchKey:     "batch-1",
		BatchSubject: "2 background agents launched",
		BatchTotal:   2,
	})
	store.UpdateBatch(Batch{
		ID:        "batch-1",
		Key:       "batch-1",
		Subject:   "2 background agents launched",
		Status:    "completed",
		Completed: 2,
		Total:     2,
		Failures:  1,
	})
	store.QueuePendingMessage("task-1", "follow up")

	snapshot, ok := store.Snapshot("task-1", "", "completed", 1)
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snapshot.Worker.PendingMessageCount != 1 {
		t.Fatalf("pending count = %d, want 1", snapshot.Worker.PendingMessageCount)
	}
	if snapshot.Decision.Phase != "completed_batch_with_failures" {
		t.Fatalf("phase = %q", snapshot.Decision.Phase)
	}
	if !snapshot.Decision.ShouldSpawnVerifier {
		t.Fatal("expected verifier suggestion")
	}
}

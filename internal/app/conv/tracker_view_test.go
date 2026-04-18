package conv

import (
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestRenderTrackerListGroupsBackgroundBatchChildren(t *testing.T) {
	tracker.Default().Reset()
	orchestration.Default().Reset()
	t.Cleanup(func() {
		tracker.Default().Reset()
		orchestration.Default().Reset()
	})

	batch := tracker.Default().Create("2 background agents launched", "", "", map[string]any{
		"background_kind":      "batch",
		"background_completed": 1,
		"background_total":     2,
		"background_failures":  0,
	})
	_ = tracker.Default().Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child1 := tracker.Default().Create("dir-audit: Directory structure audit", "", "", map[string]any{
		"background_kind":      "worker",
		"background_parent_id": batch.ID,
		"background_task_id":   "bg-1",
	})
	_ = tracker.Default().Update(child1.ID, tracker.WithStatus(tracker.StatusInProgress))

	child2 := tracker.Default().Create("naming-audit: Package naming audit", "", "", map[string]any{
		"background_kind":          "worker",
		"background_parent_id":     batch.ID,
		"background_task_id":       "bg-2",
		"background_status_detail": "failed",
	})
	_ = tracker.Default().Update(child2.ID, tracker.WithStatus(tracker.StatusCompleted))

	regular := tracker.Default().Create("Write tests", "", "", nil)
	_ = tracker.Default().Update(regular.ID, tracker.WithStatus(tracker.StatusPending))

	view := RenderTrackerList(TrackerListParams{
		Tasks:        tracker.Default().List(),
		AllDone:      tracker.Default().AllDone(),
		StreamActive: true,
		Width:        120,
		SpinnerView:  "•",
		Blockers:     tracker.Default().OpenBlockers,
		WorkerSnap: func(taskID, agentID string) (*orchestration.Snapshot, bool) {
			return orchestration.Default().Snapshot(taskID, agentID, "", 1)
		},
	})

	for _, want := range []string{
		"2 background agents launched",
		"(1/2)",
		"dir-audit: Directory structure audit",
		"naming-audit: Package naming audit",
		"[failed]",
		"Write tests",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered tracker list missing %q:\n%s", want, view)
		}
	}

	if strings.Index(view, "2 background agents launched") > strings.Index(view, "dir-audit: Directory structure audit") {
		t.Fatalf("expected batch header before child rows:\n%s", view)
	}
}

func TestRenderTrackerListPrefersOrchestrationSnapshotForBatchAndQueueState(t *testing.T) {
	tracker.Default().Reset()
	orchestration.Default().Reset()
	t.Cleanup(func() {
		tracker.Default().Reset()
		orchestration.Default().Reset()
	})

	batch := tracker.Default().Create("stale batch title", "", "", map[string]any{
		"background_kind":      "batch",
		"background_completed": 0,
		"background_total":     2,
		"background_failures":  0,
	})
	_ = tracker.Default().Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))

	child := tracker.Default().Create("dir-audit: Directory structure audit", "", "", map[string]any{
		"background_kind":      "worker",
		"background_parent_id": batch.ID,
		"background_task_id":   "bg-1",
		"background_agent_id":  "agent-1",
	})
	_ = tracker.Default().Update(child.ID, tracker.WithStatus(tracker.StatusInProgress))

	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:       "bg-1",
		AgentID:      "agent-1",
		AgentType:    "Explore",
		AgentName:    "dir-audit",
		Description:  "Directory structure audit",
		Status:       "running",
		Running:      true,
		BatchID:      batch.ID,
		BatchKey:     "call-1,call-2",
		BatchSubject: "2 background agents launched",
		BatchTotal:   2,
	})
	orchestration.Default().UpdateBatch(orchestration.Batch{
		ID:        batch.ID,
		Key:       "call-1,call-2",
		Subject:   "2 background agents launched",
		Status:    tracker.StatusInProgress,
		Completed: 1,
		Total:     2,
		Failures:  1,
		Workers: []orchestration.BatchWorker{
			{TaskID: "bg-1", Subject: "dir-audit: Directory structure audit", Status: "running", AgentType: "Explore"},
		},
	})
	orchestration.Default().QueuePendingMessage("bg-1", "Please focus on import paths")

	view := RenderTrackerList(TrackerListParams{
		Tasks:        tracker.Default().List(),
		AllDone:      tracker.Default().AllDone(),
		StreamActive: true,
		Width:        120,
		SpinnerView:  "•",
		Blockers:     tracker.Default().OpenBlockers,
		WorkerSnap: func(taskID, agentID string) (*orchestration.Snapshot, bool) {
			return orchestration.Default().Snapshot(taskID, agentID, "", 1)
		},
	})

	for _, want := range []string{
		"2 background agents launched",
		"(1/2)",
		"1 failed",
		"[1 queued]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered tracker list missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "stale batch title") {
		t.Fatalf("expected orchestration subject to override stale tracker title:\n%s", view)
	}
}

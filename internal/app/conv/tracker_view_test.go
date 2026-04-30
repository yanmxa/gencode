package conv

import (
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/task/tracker"
)

func TestRenderTrackerListShowsTaskStatus(t *testing.T) {
	tracker.Initialize(tracker.Options{})
	t.Cleanup(func() { tracker.Default().Reset() })

	inProgress := tracker.Default().Create("Fix auth module", "", "", map[string]any{
		"background_task_id":       "bg-1",
		"background_status_detail": "running",
	})
	_ = tracker.Default().Update(inProgress.ID, tracker.WithStatus(tracker.StatusInProgress))

	failed := tracker.Default().Create("Fix payment module", "", "", map[string]any{
		"background_task_id":       "bg-2",
		"background_status_detail": "failed",
	})
	_ = tracker.Default().Update(failed.ID, tracker.WithStatus(tracker.StatusCompleted))

	completed := tracker.Default().Create("Ship feature", "", "", nil)
	_ = tracker.Default().Update(completed.ID, tracker.WithStatus(tracker.StatusCompleted))

	pending := tracker.Default().Create("Write tests", "", "", nil)
	_ = tracker.Default().Update(pending.ID, tracker.WithStatus(tracker.StatusPending))

	view := RenderTrackerList(TrackerListParams{
		Tasks:        tracker.Default().List(),
		AllDone:      false,
		StreamActive: true,
		Width:        120,
		SpinnerView:  "•",
		Blockers:     tracker.Default().OpenBlockers,
	})

	for _, want := range []string{
		"●",
		"Fix auth module",
		"!",
		"Fix payment module",
		"[failed]",
		"●",
		"Ship feature",
		"○",
		"Write tests",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered tracker list missing %q:\n%s", want, view)
		}
	}
}

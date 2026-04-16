package agent

import (
	"github.com/yanmxa/gencode/internal/app/output/render"
)

// RenderTrackerList renders the task tracker list.
// Returns empty string when showTasks is false.
func RenderTrackerList(showTasks bool, streamActive bool, width int, spinnerView string) string {
	if !showTasks {
		return ""
	}
	return render.RenderTrackerList(render.TrackerListParams{
		StreamActive: streamActive,
		Width:        width,
		SpinnerView:  spinnerView,
	})
}

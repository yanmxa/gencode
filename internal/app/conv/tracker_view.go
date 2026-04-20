package conv

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

const maxVisibleTasks = 8

// TrackerListParams holds the parameters for rendering a tracker list.
type TrackerListParams struct {
	Tasks        []*tracker.Task
	AllDone      bool
	StreamActive bool
	Width        int
	SpinnerView  string
	Blockers     func(taskID string) []string
}

// RenderTrackerList renders a compact task list above the input area.
// Returns empty string when there are no tasks or all are completed and idle.
func RenderTrackerList(params TrackerListParams) string {
	if len(params.Tasks) == 0 {
		return ""
	}

	if params.AllDone && !params.StreamActive {
		return ""
	}

	completed := 0
	for _, t := range params.Tasks {
		if t.Status == tracker.StatusCompleted {
			completed++
		}
	}

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	sb.WriteString("  " + headerStyle.Render("Tasks") + " " + mutedStyle.Render(fmt.Sprintf("(%d/%d)", completed, len(params.Tasks))) + "\n")

	rendered := 0
	for _, t := range params.Tasks {
		if rendered >= maxVisibleTasks && t.Status != tracker.StatusInProgress {
			continue
		}
		sb.WriteString(renderTask(t, params.Width, params.SpinnerView, params.Blockers))
		rendered++
	}

	return sb.String()
}

func renderTask(t *tracker.Task, width int, spinnerView string, blockers func(string) []string) string {
	indent := "  "
	idTag := fmt.Sprintf("#%s ", t.ID)
	maxTextLen := width - len(indent) - len(idTag) - 6
	subject := kit.TruncateText(t.Subject, maxTextLen)
	mutedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	idStr := mutedStyle.Render(idTag)
	statusDetail := kit.MapString(t.Metadata, "background_status_detail")

	switch t.Status {
	case tracker.StatusCompleted:
		if statusDetail == "failed" || statusDetail == "killed" {
			failedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
			return indent + failedStyle.Render("!") + " " + idStr + failedStyle.Render(subject) + " " + mutedStyle.Render("["+statusDetail+"]") + "\n"
		}
		return indent + trackerCompletedStyle.Render("✓") + " " + idStr + trackerCompletedStyle.Render(subject) + "\n"

	case tracker.StatusInProgress:
		displayText := subject
		if t.ActiveForm != "" {
			displayText = kit.TruncateText(t.ActiveForm, maxTextLen)
		}
		line := indent + trackerInProgressStyle.Render(spinnerView) + " " + idStr + trackerInProgressStyle.Render(displayText)
		if elapsed := formatElapsedTime(t.StatusChangedAt); elapsed != "" {
			line += " " + mutedStyle.Render(elapsed)
		}
		return line + "\n"

	default:
		if blockers != nil {
			if bl := blockers(t.ID); len(bl) > 0 {
				blockerRefs := make([]string, len(bl))
				for i, b := range bl {
					blockerRefs[i] = "#" + b
				}
				blockedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
				suffix := " " + blockedStyle.Render("← "+strings.Join(blockerRefs, ", "))
				return indent + trackerPendingStyle.Render("○") + " " + idStr + trackerPendingStyle.Render(subject) + suffix + "\n"
			}
		}
		return indent + trackerPendingStyle.Render("○") + " " + idStr + trackerPendingStyle.Render(subject) + "\n"
	}
}

func formatElapsedTime(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	d := time.Since(since)
	if d < time.Second {
		return ""
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

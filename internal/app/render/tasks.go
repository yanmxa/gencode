// Task list rendering: displays task progress above the input area.
package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// MaxVisibleTasks is the maximum number of tasks shown before collapsing.
const MaxVisibleTasks = 8

// TodoListParams holds the parameters for rendering a todo list.
type TodoListParams struct {
	StreamActive bool
	Width        int
	SpinnerView  string
}

// RenderTodoList renders a compact task list above the input area.
// Shows all tasks including completed ones. Resets store when all done and idle.
func RenderTodoList(params TodoListParams) string {
	tasks := tool.DefaultTodoStore.List()
	if len(tasks) == 0 {
		return ""
	}

	completed := 0
	for _, t := range tasks {
		if t.Status == tool.TodoStatusCompleted {
			completed++
		}
	}

	// Reset store when all tasks completed and LLM is idle.
	if completed == len(tasks) && !params.StreamActive {
		tool.DefaultTodoStore.Reset()
		return ""
	}

	total := len(tasks)

	var sb strings.Builder

	// Header: Tasks (2/4)
	headerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	sb.WriteString("  " + headerStyle.Render("Tasks") + " " + mutedStyle.Render(fmt.Sprintf("(%d/%d)", completed, total)) + "\n")

	sb.WriteString(renderTasksFlat(tasks, params.Width, params.SpinnerView))

	return sb.String()
}

// renderTasksFlat renders tasks in a flat list (no grouping).
func renderTasksFlat(tasks []*tool.TodoTask, width int, spinnerView string) string {
	var sb strings.Builder
	shown := 0
	for _, t := range tasks {
		if shown >= MaxVisibleTasks {
			remaining := len(tasks) - shown
			moreStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
			sb.WriteString(moreStyle.Render(fmt.Sprintf("    +%d more\n", remaining)))
			break
		}
		sb.WriteString(RenderTodoTask(t, width, spinnerView))
		shown++
	}
	return sb.String()
}

// RenderTodoTask renders a single task line.
func RenderTodoTask(t *tool.TodoTask, width int, spinnerView string) string {
	return renderTodoTaskIndented(t, width, spinnerView, "")
}

// renderTodoTaskIndented renders a single task line with optional extra indentation.
func renderTodoTaskIndented(t *tool.TodoTask, width int, spinnerView string, extraIndent string) string {
	indent := extraIndent + "  "
	idTag := fmt.Sprintf("#%s ", t.ID)
	maxTextLen := width - len(indent) - len(idTag) - 6 // icon + spaces + margin
	subject := TruncateText(t.Subject, maxTextLen)

	mutedStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	idStr := mutedStyle.Render(idTag)

	switch t.Status {
	case tool.TodoStatusCompleted:
		return indent + TodoCompletedStyle.Render("✓") + " " + idStr + TodoCompletedStyle.Render(subject) + "\n"

	case tool.TodoStatusInProgress:
		displayText := subject
		if t.ActiveForm != "" {
			displayText = TruncateText(t.ActiveForm, maxTextLen)
		}
		line := indent + TodoInProgressStyle.Render(spinnerView) + " " + idStr + TodoInProgressStyle.Render(displayText)
		if elapsed := formatElapsedTime(t.StatusChangedAt); elapsed != "" {
			line += " " + mutedStyle.Render(elapsed)
		}
		return line + "\n"

	default:
		if blockers := tool.DefaultTodoStore.OpenBlockers(t.ID); len(blockers) > 0 {
			blockerRefs := make([]string, len(blockers))
			for i, b := range blockers {
				blockerRefs[i] = "#" + b
			}
			blockedStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Error)
			suffix := " " + blockedStyle.Render("← "+strings.Join(blockerRefs, ", "))
			return indent + TodoPendingStyle.Render("○") + " " + idStr + TodoPendingStyle.Render(subject) + suffix + "\n"
		}
		return indent + TodoPendingStyle.Render("○") + " " + idStr + TodoPendingStyle.Render(subject) + "\n"
	}
}

// formatElapsedTime returns a human-readable elapsed time string since the given time.
// Returns empty string if the time is zero.
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

// TruncateText shortens text to maxLen with ellipsis if needed.
func TruncateText(text string, maxLen int) string {
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return text[:maxLen]
	}
	return text[:maxLen-3] + "..."
}

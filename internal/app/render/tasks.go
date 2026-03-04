// Task list rendering: displays task progress above the input area.
package render

import (
	"fmt"
	"strings"

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

	// Reset store when all tasks completed and LLM is idle
	if completed == len(tasks) && !params.StreamActive {
		tool.DefaultTodoStore.Reset()
		return ""
	}

	var sb strings.Builder

	// Progress header
	total := len(tasks)
	progressStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	headerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent).Bold(true)
	sb.WriteString(headerStyle.Render("  Tasks") + " ")
	sb.WriteString(progressStyle.Render(fmt.Sprintf("%d/%d", completed, total)))
	sb.WriteString("\n")

	// Render all tasks (up to MaxVisibleTasks)
	shown := 0
	for _, t := range tasks {
		if shown >= MaxVisibleTasks {
			remaining := total - shown
			moreStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
			sb.WriteString(moreStyle.Render(fmt.Sprintf("  ... and %d more\n", remaining)))
			break
		}

		sb.WriteString(RenderTodoTask(t, params.Width, params.SpinnerView))
		shown++
	}

	return sb.String()
}

// RenderTodoTask renders a single task line.
func RenderTodoTask(t *tool.TodoTask, width int, spinnerView string) string {
	subject := TruncateText(t.Subject, width-6)

	switch t.Status {
	case tool.TodoStatusCompleted:
		return TodoCompletedStyle.Render("  ✓ "+subject) + "\n"

	case tool.TodoStatusInProgress:
		return TodoInProgressStyle.Render("  "+spinnerView+" "+subject) + "\n"

	default:
		if tool.DefaultTodoStore.IsBlocked(t.ID) {
			blockedStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDisabled)
			return blockedStyle.Render("  ▸ "+subject) + "\n"
		}
		return TodoPendingStyle.Render("  ☐ "+subject) + "\n"
	}
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

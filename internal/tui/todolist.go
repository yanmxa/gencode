package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool"
)

// maxVisibleTasks is the maximum number of tasks shown before collapsing
const maxVisibleTasks = 8

// renderTodoList renders a compact task list above the input area.
// Shows all tasks including completed ones. Resets store when all done and idle.
func (m model) renderTodoList() string {
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
	if completed == len(tasks) && !m.streaming {
		tool.DefaultTodoStore.Reset()
		return ""
	}

	var sb strings.Builder

	// Progress header
	total := len(tasks)
	progressStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	headerStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Bold(true)
	sb.WriteString(headerStyle.Render("  Tasks") + " ")
	sb.WriteString(progressStyle.Render(fmt.Sprintf("%d/%d", completed, total)))
	sb.WriteString("\n")

	// Render all tasks (up to maxVisibleTasks)
	shown := 0
	for _, t := range tasks {
		if shown >= maxVisibleTasks {
			remaining := total - shown
			moreStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
			sb.WriteString(moreStyle.Render(fmt.Sprintf("  ... and %d more\n", remaining)))
			break
		}

		sb.WriteString(m.renderTodoTask(t))
		shown++
	}

	return sb.String()
}

// renderTodoTask renders a single task line
func (m model) renderTodoTask(t *tool.TodoTask) string {
	subject := m.truncateText(t.Subject, m.width-6)

	switch t.Status {
	case tool.TodoStatusCompleted:
		return todoCompletedStyle.Render("  ✓ "+subject) + "\n"

	case tool.TodoStatusInProgress:
		line := todoInProgressStyle.Render("  "+m.spinner.View()+" "+subject) + "\n"
		if t.ActiveForm != "" {
			form := m.truncateText(t.ActiveForm, m.width-6)
			activeStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Accent)
			line += activeStyle.Render("    "+form) + "\n"
		}
		return line

	default:
		if tool.DefaultTodoStore.IsBlocked(t.ID) {
			blockedStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDisabled)
			return blockedStyle.Render("  ▸ "+subject) + "\n"
		}
		return todoPendingStyle.Render("  ☐ "+subject) + "\n"
	}
}

// truncateText shortens text to maxLen with ellipsis if needed
func (m model) truncateText(text string, maxLen int) string {
	if maxLen > 0 && len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TodoPanel displays the current task list
type TodoPanel struct {
	todos []ui.TodoItem
	width int
}

// NewTodoPanel creates a new TodoPanel
func NewTodoPanel() *TodoPanel {
	return &TodoPanel{
		todos: []ui.TodoItem{},
		width: 60,
	}
}

// SetWidth sets the panel width
func (p *TodoPanel) SetWidth(width int) {
	p.width = width
	if p.width < 30 {
		p.width = 30
	}
	if p.width > 80 {
		p.width = 80
	}
}

// Update updates the todo list
func (p *TodoPanel) Update(todos []ui.TodoItem) {
	p.todos = todos
}

// IsVisible returns true if there are todos to display
func (p *TodoPanel) IsVisible() bool {
	return len(p.todos) > 0
}

// Clear clears all todos
func (p *TodoPanel) Clear() {
	p.todos = []ui.TodoItem{}
}

// Styles for the todo panel (shared styles are in app.go)
var (
	todoHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")) // muted gray
)

// RenderInline renders the todo panel inline (not as overlay)
// Format:
//
//	📋 Tasks [1/4]
//	  completed task (strikethrough)
//	  in progress task (orange highlight)
//	  pending task (gray)
//
// Order: completed → in_progress → pending
func (p *TodoPanel) RenderInline() string {
	if !p.IsVisible() {
		return ""
	}

	var sb strings.Builder

	// Count tasks for progress display
	pending, inProgress, completed := 0, 0, 0
	for _, todo := range p.todos {
		switch todo.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "completed":
			completed++
		}
	}
	total := pending + inProgress + completed

	// Header line with clipboard icon: 📋 Tasks [2/5]
	header := todoHeaderStyle.Render(fmt.Sprintf(" 📋 Tasks [%d/%d]", completed, total))
	sb.WriteString(header + "\n")

	// 2-space indent to align with header
	indent := "  "

	// Render in order: completed → in_progress → pending
	for _, todo := range p.todos {
		if todo.Status == "completed" {
			sb.WriteString(indent + todoCompletedStyle.Render(todo.Content) + "\n")
		}
	}
	for _, todo := range p.todos {
		if todo.Status == "in_progress" {
			sb.WriteString(indent + todoInProgressStyle.Render(todo.ActiveForm) + "\n")
		}
	}
	for _, todo := range p.todos {
		if todo.Status == "pending" {
			sb.WriteString(indent + todoPendingStyle.Render(todo.Content) + "\n")
		}
	}

	return sb.String()
}

// renderTodoLine renders a single todo item
func (p *TodoPanel) renderTodoLine(todo ui.TodoItem) string {
	indent := "      " // 6 spaces to align with header

	switch todo.Status {
	case "completed":
		// Strikethrough style for completed
		return indent + todoCompletedStyle.Render(todo.Content)
	case "in_progress":
		// Arrow prefix and highlight for in-progress
		return indent + todoInProgressStyle.Render("→ "+todo.ActiveForm)
	default: // pending
		return indent + todoPendingStyle.Render(todo.Content)
	}
}

// Render renders the todo panel (deprecated, use RenderInline)
func (p *TodoPanel) Render() string {
	return p.RenderInline()
}

// RenderCompact renders a compact single-line summary
func (p *TodoPanel) RenderCompact() string {
	if !p.IsVisible() {
		return ""
	}

	pending, inProgress, completed := 0, 0, 0
	var currentTask string

	for _, todo := range p.todos {
		switch todo.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
			if currentTask == "" {
				currentTask = todo.ActiveForm
			}
		case "completed":
			completed++
		}
	}

	total := pending + inProgress + completed
	progress := fmt.Sprintf("[%d/%d]", completed, total)

	if currentTask != "" {
		// Truncate if too long
		maxLen := 40
		if len(currentTask) > maxLen {
			currentTask = currentTask[:maxLen-3] + "..."
		}
		return fmt.Sprintf("  📋 Tasks %s %s", progress, todoInProgressStyle.Render(currentTask))
	}

	return fmt.Sprintf("  📋 Tasks %s", progress)
}

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

// Styles for the todo panel
var (
	todoPanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#60A5FA")). // blue
				Padding(0, 1)

	todoHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	todoPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")) // gray

	todoInProgressStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B")). // orange
				Bold(true)

	todoCompletedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")) // green
)

// Status icons
const (
	iconPending    = "⬜"
	iconInProgress = "🔄"
	iconCompleted  = "✅"
)

// Render renders the todo panel
func (p *TodoPanel) Render() string {
	if !p.IsVisible() {
		return ""
	}

	var sb strings.Builder

	// Calculate content width (panel width minus border and padding)
	contentWidth := p.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Header
	header := todoHeaderStyle.Render("📋 Tasks")
	sb.WriteString(header)
	sb.WriteString("\n")

	// Render each todo
	for _, todo := range p.todos {
		icon, style, text := getTodoDisplay(todo)

		// Truncate if too long
		maxTextLen := contentWidth - 4 // icon + space + some padding
		if len(text) > maxTextLen {
			text = text[:maxTextLen-3] + "..."
		}

		line := fmt.Sprintf("%s %s", icon, style.Render(text))
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Remove trailing newline
	content := strings.TrimSuffix(sb.String(), "\n")

	// Apply border
	return todoPanelBorderStyle.Width(p.width).Render(content)
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
		return fmt.Sprintf("📋 %s %s", progress, todoInProgressStyle.Render(currentTask))
	}

	return fmt.Sprintf("📋 %s", progress)
}

// getTodoDisplay returns the icon, style, and text for a todo item
func getTodoDisplay(todo ui.TodoItem) (string, lipgloss.Style, string) {
	switch todo.Status {
	case "in_progress":
		return iconInProgress, todoInProgressStyle, todo.ActiveForm
	case "completed":
		return iconCompleted, todoCompletedStyle, todo.Content
	default: // "pending" or unknown
		return iconPending, todoPendingStyle, todo.Content
	}
}

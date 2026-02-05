package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/system"
)

// MemoryItem represents a memory file option in the selector
type MemoryItem struct {
	Label       string // Display label (e.g., "Global", "Project", "Local")
	Description string // Description (e.g., "Saved in ~/.gen/GEN.md")
	Path        string // File path (empty if not exists)
	Exists      bool   // Whether the file exists
	Size        int64  // File size in bytes
	Level       string // "global", "project", "local"
	CreateHint  string // Command to create if not exists
}

// MemorySelectorState holds the state for the memory selector
type MemorySelectorState struct {
	active      bool
	items       []MemoryItem
	selectedIdx int
	width       int
	height      int
	cwd         string
}

// MemorySelectedMsg is sent when a memory file is selected for editing
type MemorySelectedMsg struct {
	Path  string
	Level string
}

// MemorySelectorCancelledMsg is sent when the selector is cancelled
type MemorySelectorCancelledMsg struct{}

// NewMemorySelectorState creates a new MemorySelectorState
func NewMemorySelectorState() MemorySelectorState {
	return MemorySelectorState{
		active:      false,
		items:       []MemoryItem{},
		selectedIdx: 0,
	}
}

// EnterMemorySelect enters memory selection mode
func (s *MemorySelectorState) EnterMemorySelect(cwd string, width, height int) {
	s.cwd = cwd
	s.width = width
	s.height = height
	s.active = true
	s.selectedIdx = 0

	paths := system.GetAllMemoryPaths(cwd)
	s.items = []MemoryItem{
		s.buildMemoryItem("Global", "global", paths.Global, cwd,
			fmt.Sprintf("Saved in %s", shortenPath(paths.Global[0])),
			"Will be created on edit"),

		s.buildMemoryItem("Project", "project", paths.Project, cwd,
			"Checked in at .gen/GEN.md",
			"Use /init to create"),

		s.buildMemoryItem("Local", "local", paths.Local, cwd,
			"Not committed (git-ignored)",
			"Use /init local to create"),
	}
}

// buildMemoryItem creates a MemoryItem from the given parameters
func (s *MemorySelectorState) buildMemoryItem(label, level string, searchPaths []string, cwd, defaultDesc, createHint string) MemoryItem {
	foundPath := system.FindMemoryFile(searchPaths)
	exists := foundPath != ""

	path := foundPath
	if !exists {
		path = searchPaths[0]
	}

	description := defaultDesc
	if exists && level == "project" {
		description = fmt.Sprintf("Checked in at %s", shortenPathForProject(foundPath, cwd))
	}

	return MemoryItem{
		Label:       label,
		Description: description,
		Path:        path,
		Exists:      exists,
		Size:        system.GetFileSize(path),
		Level:       level,
		CreateHint:  createHint,
	}
}

// shortenPathForProject returns a relative path if within project, otherwise shortened
func shortenPathForProject(path, cwd string) string {
	if strings.HasPrefix(path, cwd) {
		rel := strings.TrimPrefix(path, cwd)
		rel = strings.TrimPrefix(rel, "/")
		if rel != "" {
			return rel
		}
	}
	return shortenPath(path)
}

// IsActive returns whether the selector is active
func (s *MemorySelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *MemorySelectorState) Cancel() {
	s.active = false
	s.items = []MemoryItem{}
	s.selectedIdx = 0
}

// MoveUp moves the selection up
func (s *MemorySelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
	}
}

// MoveDown moves the selection down
func (s *MemorySelectorState) MoveDown() {
	if s.selectedIdx < len(s.items)-1 {
		s.selectedIdx++
	}
}

// Select handles selection and returns a command
func (s *MemorySelectorState) Select() tea.Cmd {
	if s.selectedIdx >= len(s.items) {
		return nil
	}

	selected := s.items[s.selectedIdx]
	s.active = false

	return func() tea.Msg {
		return MemorySelectedMsg{
			Path:  selected.Path,
			Level: selected.Level,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if selection is made
func (s *MemorySelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	keyStr := key.String()

	// Navigation: up/down movement
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter, tea.KeyRight:
		return s.Select()
	case tea.KeyEsc, tea.KeyLeft:
		return s.cancelWithMsg()
	}

	// Vim-style navigation and quick select
	switch keyStr {
	case "j":
		s.MoveDown()
	case "k":
		s.MoveUp()
	case "l":
		return s.Select()
	case "h":
		return s.cancelWithMsg()
	case "1", "2", "3":
		idx := int(keyStr[0] - '1')
		if idx < len(s.items) {
			s.selectedIdx = idx
			return s.Select()
		}
	}

	return nil
}

// cancelWithMsg cancels the selector and returns a cancelled message
func (s *MemorySelectorState) cancelWithMsg() tea.Cmd {
	s.Cancel()
	return func() tea.Msg {
		return MemorySelectorCancelledMsg{}
	}
}

// Render renders the selector
func (s *MemorySelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title
	sb.WriteString(selectorTitleStyle.Render("Select memory to edit:"))
	sb.WriteString("\n\n")

	// Render items
	for i, item := range s.items {
		// Status icon
		var statusIcon string
		var statusStyle lipgloss.Style

		if item.Exists {
			statusIcon = "●"
			statusStyle = selectorStatusConnected
		} else {
			statusIcon = "○"
			statusStyle = selectorStatusNone
		}

		// Format: [1] ● Label     Description (Size)
		numKey := fmt.Sprintf("%d.", i+1)
		sizeStr := ""
		if item.Exists && item.Size > 0 {
			sizeStr = fmt.Sprintf(" (%s)", system.FormatFileSize(item.Size))
		}

		// Build the line
		line := fmt.Sprintf("%s %s %s",
			statusStyle.Render(statusIcon),
			item.Label,
			selectorHintStyle.Render(item.Description+sizeStr),
		)

		if i == s.selectedIdx {
			sb.WriteString(selectorSelectedStyle.Render(fmt.Sprintf("❯ %s %s", numKey, line)))
		} else {
			sb.WriteString(selectorItemStyle.Render(fmt.Sprintf("  %s %s", numKey, line)))
		}
		sb.WriteString("\n")

		// Show creation hint for non-existing files
		if !item.Exists && i == s.selectedIdx {
			sb.WriteString(selectorItemStyle.Render("      " + selectorHintStyle.Render(item.CreateHint)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selectorHintStyle.Render("↑/↓ navigate · Enter edit · 1-3 quick select · Esc cancel"))

	// Wrap in border
	content := sb.String()
	box := selectorBorderStyle.Width(calculateBoxWidth(s.width)).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

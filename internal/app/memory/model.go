// Package memory provides the memory file selector feature.
package memory

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

// Item represents a memory file option in the selector.
type Item struct {
	Label       string
	Description string
	Path        string
	Exists      bool
	Size        int64
	Level       string
	CreateHint  string
}

// Model holds the state for the memory selector.
type Model struct {
	active      bool
	items       []Item
	selectedIdx int
	width       int
	height      int
	cwd         string
}

// SelectedMsg is sent when a memory file is selected for editing.
type SelectedMsg struct {
	Path  string
	Level string
}

// New creates a new memory selector Model.
func New() Model {
	return Model{
		active:      false,
		items:       []Item{},
		selectedIdx: 0,
	}
}

// EnterSelect enters memory selection mode.
func (m *Model) EnterSelect(cwd string, width, height int) {
	m.cwd = cwd
	m.width = width
	m.height = height
	m.active = true
	m.selectedIdx = 0

	paths := system.GetAllMemoryPaths(cwd)
	m.items = []Item{
		m.buildItem("Global", "global", paths.Global, cwd,
			fmt.Sprintf("Saved in %s", shared.ShortenPath(paths.Global[0])),
			"Will be created on edit"),

		m.buildItem("Project", "project", paths.Project, cwd,
			"Checked in at .gen/GEN.md",
			"Use /init to create"),

		m.buildItem("Local", "local", paths.Local, cwd,
			"Not committed (git-ignored)",
			"Use /init local to create"),
	}
}

func (m *Model) buildItem(label, level string, searchPaths []string, cwd, defaultDesc, createHint string) Item {
	foundPath := system.FindMemoryFile(searchPaths)
	exists := foundPath != ""

	path := foundPath
	if !exists {
		path = searchPaths[0]
	}

	description := defaultDesc
	if exists && level == "project" {
		description = fmt.Sprintf("Checked in at %s", shared.ShortenPathForProject(foundPath, cwd))
	}

	return Item{
		Label:       label,
		Description: description,
		Path:        path,
		Exists:      exists,
		Size:        system.GetFileSize(path),
		Level:       level,
		CreateHint:  createHint,
	}
}

// IsActive returns whether the selector is active.
func (m *Model) IsActive() bool {
	return m.active
}

// Cancel cancels the selector.
func (m *Model) Cancel() {
	m.active = false
	m.items = []Item{}
	m.selectedIdx = 0
}

// HandleKeypress handles a keypress and returns a command if selection is made.
func (m *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	keyStr := key.String()

	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		if m.selectedIdx < len(m.items)-1 {
			m.selectedIdx++
		}
		return nil
	case tea.KeyEnter, tea.KeyRight:
		return m.selectItem()
	case tea.KeyEsc, tea.KeyLeft:
		return m.cancelWithMsg()
	}

	switch keyStr {
	case "j":
		if m.selectedIdx < len(m.items)-1 {
			m.selectedIdx++
		}
	case "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "l":
		return m.selectItem()
	case "h":
		return m.cancelWithMsg()
	case "1", "2", "3":
		idx := int(keyStr[0] - '1')
		if idx < len(m.items) {
			m.selectedIdx = idx
			return m.selectItem()
		}
	}

	return nil
}

func (m *Model) selectItem() tea.Cmd {
	if m.selectedIdx >= len(m.items) {
		return nil
	}

	selected := m.items[m.selectedIdx]
	m.active = false

	return func() tea.Msg {
		return SelectedMsg{
			Path:  selected.Path,
			Level: selected.Level,
		}
	}
}

func (m *Model) cancelWithMsg() tea.Cmd {
	m.Cancel()
	return func() tea.Msg {
		return shared.DismissedMsg{}
	}
}

// Render renders the selector.
func (m *Model) Render() string {
	if !m.active {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(shared.SelectorTitleStyle.Render("Select memory to edit:"))
	sb.WriteString("\n\n")

	for i, item := range m.items {
		var statusIcon string
		var statusStyle lipgloss.Style

		if item.Exists {
			statusIcon = "●"
			statusStyle = shared.SelectorStatusConnected
		} else {
			statusIcon = "○"
			statusStyle = shared.SelectorStatusNone
		}

		numKey := fmt.Sprintf("%d.", i+1)
		sizeStr := ""
		if item.Exists && item.Size > 0 {
			sizeStr = fmt.Sprintf(" (%s)", system.FormatFileSize(item.Size))
		}

		line := fmt.Sprintf("%s %s %s",
			statusStyle.Render(statusIcon),
			item.Label,
			shared.SelectorHintStyle.Render(item.Description+sizeStr),
		)

		if i == m.selectedIdx {
			sb.WriteString(shared.SelectorSelectedStyle.Render(fmt.Sprintf("❯ %s %s", numKey, line)))
		} else {
			sb.WriteString(shared.SelectorItemStyle.Render(fmt.Sprintf("  %s %s", numKey, line)))
		}
		sb.WriteString("\n")

		if !item.Exists && i == m.selectedIdx {
			sb.WriteString(shared.SelectorItemStyle.Render("      " + shared.SelectorHintStyle.Render(item.CreateHint)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(shared.SelectorHintStyle.Render("↑/↓ navigate · Enter edit · 1-3 quick select · Esc cancel"))

	content := sb.String()
	box := shared.SelectorBorderStyle.Width(shared.CalculateBoxWidth(m.width)).Render(content)

	return lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, box)
}

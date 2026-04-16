// Package tool provides the tool selector feature.
package toolui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/app/ui/selector"
	"github.com/yanmxa/gencode/internal/app/ui/theme"
)

// item represents a tool in the selector
type item struct {
	Name        string
	Description string
	Enabled     bool
}

// SaveLevel represents where to save the tool settings
type SaveLevel int

const (
	saveLevelProject SaveLevel = iota // Save to .gen/settings.json
	saveLevelUser                     // Save to ~/.gen/settings.json
)

// String returns the display name for the save level
func (l SaveLevel) String() string {
	if l == saveLevelUser {
		return "User"
	}
	return "Project"
}

// Model holds state for the tool selector
type Model struct {
	active        bool
	tools         []item // All tools
	filteredTools []item // Filtered tools based on search
	selectedIdx   int
	width         int
	height        int
	searchQuery   string // Fuzzy search query
	scrollOffset  int    // Scroll offset for visible items
	maxVisible    int    // Maximum visible items (default 10)

	disabledTools map[string]bool // Reference to disabled tools map
	saveLevel     SaveLevel       // Where to save settings (project or user)
}

// ToggleMsg is sent when a tool's enabled state is toggled
type ToggleMsg struct {
	ToolName string
	Enabled  bool
}

// New creates a new Model
func New() Model {
	return Model{
		active:     false,
		tools:      []item{},
		maxVisible: 10,
	}
}

// EnterSelect enters tool selection mode
func (s *Model) EnterSelect(width, height int, disabledTools map[string]bool, mcpTools func() []core.ToolSchema) error {
	// Get all tool schemas including MCP tools
	allTools := coretool.GetToolSchemasWithMCP(mcpTools)

	s.tools = make([]item, 0, len(allTools))
	for _, t := range allTools {
		s.tools = append(s.tools, item{
			Name:        t.Name,
			Description: t.Description,
			Enabled:     !disabledTools[t.Name],
		})
	}

	s.active = true
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
	s.width = width
	s.height = height
	s.disabledTools = disabledTools
	s.filteredTools = s.tools

	return nil
}

// IsActive returns whether the selector is active
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *Model) Cancel() {
	s.active = false
	s.tools = []item{}
	s.filteredTools = []item{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
}

// MoveUp moves the selection up
func (s *Model) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *Model) MoveDown() {
	if s.selectedIdx < len(s.filteredTools)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *Model) ensureVisible() {
	// Scroll up if selection is above viewport
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	// Scroll down if selection is below viewport
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters tools based on search query (fuzzy match)
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredTools = s.tools
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredTools = make([]item, 0)
		for _, t := range s.tools {
			// Fuzzy match: check if query chars appear in order
			if selector.FuzzyMatch(strings.ToLower(t.Name), query) ||
				selector.FuzzyMatch(strings.ToLower(t.Description), query) {
				s.filteredTools = append(s.filteredTools, t)
			}
		}
	}
	// Reset selection and scroll
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// reloadToolStates reloads the enabled/disabled states from the current save level
func (s *Model) reloadToolStates() {
	// Load disabled tools from the current level (not merged)
	levelDisabled := config.GetDisabledToolsAt(s.saveLevel == saveLevelUser)

	// Update disabledTools reference
	// Clear and repopulate to maintain the same map reference
	for k := range s.disabledTools {
		delete(s.disabledTools, k)
	}
	for k, v := range levelDisabled {
		s.disabledTools[k] = v
	}

	// Update tool enabled states
	for i := range s.tools {
		s.tools[i].Enabled = !s.disabledTools[s.tools[i].Name]
	}

	// Update filtered tools as well
	for i := range s.filteredTools {
		s.filteredTools[i].Enabled = !s.disabledTools[s.filteredTools[i].Name]
	}
}

// Toggle toggles the enabled state of the currently selected tool
func (s *Model) Toggle() tea.Cmd {
	if len(s.filteredTools) == 0 || s.selectedIdx >= len(s.filteredTools) {
		return nil
	}

	selected := &s.filteredTools[s.selectedIdx]
	selected.Enabled = !selected.Enabled

	// Update the source tools list
	for i := range s.tools {
		if s.tools[i].Name == selected.Name {
			s.tools[i].Enabled = selected.Enabled
			break
		}
	}

	// Update the disabledTools map
	if selected.Enabled {
		delete(s.disabledTools, selected.Name)
	} else {
		s.disabledTools[selected.Name] = true
	}

	// Save to settings (project or user level based on saveLevel)
	_ = config.UpdateDisabledToolsAt(s.disabledTools, s.saveLevel == saveLevelUser)

	return func() tea.Msg {
		return ToggleMsg{
			ToolName: selected.Name,
			Enabled:  selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyTab:
		// Toggle save level between project and user
		if s.saveLevel == saveLevelProject {
			s.saveLevel = saveLevelUser
		} else {
			s.saveLevel = saveLevelProject
		}
		// Reload tool states from the new level
		s.reloadToolStates()
		return nil
	case tea.KeyEnter:
		return s.Toggle()
	case tea.KeyEsc:
		// First clear search if active
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		// Then close the selector
		s.Cancel()
		return func() tea.Msg {
			return selector.DismissedMsg{}
		}
	case tea.KeyBackspace:
		// Handle backspace for search
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		// Handle text input for fuzzy search
		s.searchQuery += string(key.Runes)
		s.updateFilter()
		return nil
	}

	// Handle j/k for vim-style navigation (only when not searching)
	if s.searchQuery == "" {
		switch key.String() {
		case "j":
			s.MoveDown()
			return nil
		case "k":
			s.MoveUp()
			return nil
		}
	}

	return nil
}

// Render renders the tool selector
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Tools (%d/%d)  %s", len(s.filteredTools), len(s.tools), levelIndicator)
	sb.WriteString(selector.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "🔍 "
	if s.searchQuery == "" {
		sb.WriteString(selector.SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(selector.SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "▏"))
	}
	sb.WriteString("\n\n")

	// Calculate box width for dynamic description length
	boxWidth := selector.CalculateToolBoxWidth(s.width)
	// Available width = boxWidth - border(2) - padding(4) - itemPaddingLeft(2) - prefix(2) - icon(2) - name(15) - spacing(2)
	// Total overhead = 29, use 30 for safety margin
	maxDescLen := max(boxWidth-30, 20)

	// Handle empty results
	if len(s.filteredTools) == 0 {
		sb.WriteString(selector.SelectorHintStyle.Render("  No tools match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredTools))

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(selector.SelectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		// Render visible tools
		for i := s.scrollOffset; i < endIdx; i++ {
			t := s.filteredTools[i]

			// Status icon: ● enabled (green), ○ disabled (gray)
			var statusIcon string
			var statusStyle lipgloss.Style
			if t.Enabled {
				statusIcon = "●"
				statusStyle = selector.SelectorStatusConnected
			} else {
				statusIcon = "○"
				statusStyle = selector.SelectorStatusNone
			}

			// Use only the first line of description, then truncate if needed
			desc := t.Description
			if idx := strings.Index(desc, "\n"); idx != -1 {
				desc = desc[:idx]
			}
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}

			// Use inline style for description (without MarginTop from SelectorHintStyle)
			descStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s  %s",
				statusStyle.Render(statusIcon),
				t.Name,
				descStyle.Render(desc),
			)

			if i == s.selectedIdx {
				sb.WriteString(selector.SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(selector.SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredTools) {
			sb.WriteString(selector.SelectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selector.SelectorHintStyle.Render("↑/↓ navigate · Enter toggle · Tab level · Esc cancel"))

	// Wrap in border
	content := sb.String()
	box := selector.SelectorBorderStyle.Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

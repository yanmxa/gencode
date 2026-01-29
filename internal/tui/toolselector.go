package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/tool"
)

// ToolItem represents a tool in the selector
type ToolItem struct {
	Name        string
	Description string
	Enabled     bool
}

// SaveLevel represents where to save the tool settings
type SaveLevel int

const (
	SaveLevelProject SaveLevel = iota // Save to .gen/settings.json
	SaveLevelUser                     // Save to ~/.gen/settings.json
)

// String returns the display name for the save level
func (l SaveLevel) String() string {
	if l == SaveLevelUser {
		return "User"
	}
	return "Project"
}

// ToolSelectorState holds state for the tool selector
type ToolSelectorState struct {
	active        bool
	tools         []ToolItem // All tools
	filteredTools []ToolItem // Filtered tools based on search
	selectedIdx   int
	width         int
	height        int
	searchQuery   string // Fuzzy search query
	scrollOffset  int    // Scroll offset for visible items
	maxVisible    int    // Maximum visible items (default 10)

	disabledTools map[string]bool // Reference to disabled tools map
	saveLevel     SaveLevel       // Where to save settings (project or user)
}

// ToolToggleMsg is sent when a tool's enabled state is toggled
type ToolToggleMsg struct {
	ToolName string
	Enabled  bool
}

// ToolSelectorCancelledMsg is sent when the tool selector is cancelled
type ToolSelectorCancelledMsg struct{}

// NewToolSelectorState creates a new ToolSelectorState
func NewToolSelectorState() ToolSelectorState {
	return ToolSelectorState{
		active:     false,
		tools:      []ToolItem{},
		maxVisible: 10,
	}
}

// EnterToolSelect enters tool selection mode
func (s *ToolSelectorState) EnterToolSelect(width, height int, disabledTools map[string]bool) error {
	// Get all tool schemas
	allTools := tool.GetToolSchemas()

	s.tools = make([]ToolItem, 0, len(allTools))
	for _, t := range allTools {
		s.tools = append(s.tools, ToolItem{
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
func (s *ToolSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *ToolSelectorState) Cancel() {
	s.active = false
	s.tools = []ToolItem{}
	s.filteredTools = []ToolItem{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
}

// MoveUp moves the selection up
func (s *ToolSelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *ToolSelectorState) MoveDown() {
	if s.selectedIdx < len(s.filteredTools)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *ToolSelectorState) ensureVisible() {
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
func (s *ToolSelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filteredTools = s.tools
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredTools = make([]ToolItem, 0)
		for _, t := range s.tools {
			// Fuzzy match: check if query chars appear in order
			if fuzzyMatch(strings.ToLower(t.Name), query) ||
				fuzzyMatch(strings.ToLower(t.Description), query) {
				s.filteredTools = append(s.filteredTools, t)
			}
		}
	}
	// Reset selection and scroll
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// reloadToolStates reloads the enabled/disabled states from the current save level
func (s *ToolSelectorState) reloadToolStates() {
	// Load disabled tools from the current level (not merged)
	levelDisabled := config.GetDisabledToolsAt(s.saveLevel == SaveLevelUser)

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
func (s *ToolSelectorState) Toggle() tea.Cmd {
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
	_ = config.UpdateDisabledToolsAt(s.disabledTools, s.saveLevel == SaveLevelUser)

	return func() tea.Msg {
		return ToolToggleMsg{
			ToolName: selected.Name,
			Enabled:  selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *ToolSelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyTab:
		// Toggle save level between project and user
		if s.saveLevel == SaveLevelProject {
			s.saveLevel = SaveLevelUser
		} else {
			s.saveLevel = SaveLevelProject
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
			return ToolSelectorCancelledMsg{}
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

// calculateToolBoxWidth returns the constrained box width for tool selector.
// Uses a wider width than the default selector to show more description.
func calculateToolBoxWidth(screenWidth int) int {
	boxWidth := screenWidth - 8
	return max(50, min(boxWidth, 80))
}

// Render renders the tool selector
func (s *ToolSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Tools (%d/%d)  %s", len(s.filteredTools), len(s.tools), levelIndicator)
	sb.WriteString(selectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "🔍 "
	if s.searchQuery == "" {
		sb.WriteString(selectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(selectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "▏"))
	}
	sb.WriteString("\n\n")

	// Calculate box width for dynamic description length
	boxWidth := calculateToolBoxWidth(s.width)
	// Available width = boxWidth - border(2) - padding(4) - itemPaddingLeft(2) - prefix(2) - icon(2) - name(15) - spacing(2)
	// Total overhead = 29, use 30 for safety margin
	maxDescLen := max(boxWidth-30, 20)

	// Handle empty results
	if len(s.filteredTools) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No tools match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredTools))

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(selectorHintStyle.Render("  ↑ more above"))
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
				statusStyle = selectorStatusConnected
			} else {
				statusIcon = "○"
				statusStyle = selectorStatusNone
			}

			// Truncate description if too long
			desc := t.Description
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}

			// Use inline style for description (without MarginTop from selectorHintStyle)
			descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s  %s",
				statusStyle.Render(statusIcon),
				t.Name,
				descStyle.Render(desc),
			)

			if i == s.selectedIdx {
				sb.WriteString(selectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(selectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredTools) {
			sb.WriteString(selectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selectorHintStyle.Render("↑/↓ navigate · Enter toggle · Tab level · Esc cancel"))

	// Wrap in border
	content := sb.String()
	box := selectorBorderStyle.Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

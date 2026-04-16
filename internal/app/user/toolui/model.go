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
	"github.com/yanmxa/gencode/internal/app/kit"
)

// item represents a tool in the selector
type item struct {
	Name        string
	Description string
	Enabled     bool
}

// Model holds state for the tool selector
type Model struct {
	active        bool
	tools         []item // All tools
	filteredTools []item // Filtered tools based on search
	nav           kit.ListNav
	width         int
	height        int

	disabledTools map[string]bool // Reference to disabled tools map
	saveLevel     kit.SaveLevel
}

// ToggleMsg is sent when a tool's enabled state is toggled
type ToggleMsg struct {
	ToolName string
	Enabled  bool
}

// New creates a new Model
func New() Model {
	return Model{
		active: false,
		tools:  []item{},
		nav:    kit.ListNav{MaxVisible: 10},
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
	s.width = width
	s.height = height
	s.disabledTools = disabledTools
	s.filteredTools = s.tools
	s.nav.Reset()
	s.nav.Total = len(s.filteredTools)

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
	s.nav.Reset()
	s.nav.Total = 0
}

// updateFilter filters tools based on search query (fuzzy match)
func (s *Model) updateFilter() {
	if s.nav.Search == "" {
		s.filteredTools = s.tools
	} else {
		query := strings.ToLower(s.nav.Search)
		s.filteredTools = make([]item, 0)
		for _, t := range s.tools {
			if kit.FuzzyMatch(strings.ToLower(t.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(t.Description), query) {
				s.filteredTools = append(s.filteredTools, t)
			}
		}
	}
	s.nav.ResetCursor()
	s.nav.Total = len(s.filteredTools)
}

// reloadToolStates reloads the enabled/disabled states from the current save level
func (s *Model) reloadToolStates() {
	levelDisabled := config.GetDisabledToolsAt(s.saveLevel == kit.SaveLevelUser)

	for k := range s.disabledTools {
		delete(s.disabledTools, k)
	}
	for k, v := range levelDisabled {
		s.disabledTools[k] = v
	}

	for i := range s.tools {
		s.tools[i].Enabled = !s.disabledTools[s.tools[i].Name]
	}
	for i := range s.filteredTools {
		s.filteredTools[i].Enabled = !s.disabledTools[s.filteredTools[i].Name]
	}
}

// Toggle toggles the enabled state of the currently selected tool
func (s *Model) Toggle() tea.Cmd {
	if len(s.filteredTools) == 0 || s.nav.Selected >= len(s.filteredTools) {
		return nil
	}

	selected := &s.filteredTools[s.nav.Selected]
	selected.Enabled = !selected.Enabled

	for i := range s.tools {
		if s.tools[i].Name == selected.Name {
			s.tools[i].Enabled = selected.Enabled
			break
		}
	}

	if selected.Enabled {
		delete(s.disabledTools, selected.Name)
	} else {
		s.disabledTools[selected.Name] = true
	}

	_ = config.UpdateDisabledToolsAt(s.disabledTools, s.saveLevel == kit.SaveLevelUser)

	return func() tea.Msg {
		return ToggleMsg{
			ToolName: selected.Name,
			Enabled:  selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyTab {
		if s.saveLevel == kit.SaveLevelProject {
			s.saveLevel = kit.SaveLevelUser
		} else {
			s.saveLevel = kit.SaveLevelProject
		}
		s.reloadToolStates()
		return nil
	}

	if key.Type == tea.KeyEnter {
		return s.Toggle()
	}

	searchChanged, consumed := s.nav.HandleKey(key)
	if searchChanged {
		s.updateFilter()
	}
	if consumed {
		return nil
	}

	if key.Type == tea.KeyEsc {
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
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
	sb.WriteString(kit.SelectorTitleStyle().Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "🔍 "
	if s.nav.Search == "" {
		sb.WriteString(kit.SelectorHintStyle().Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(kit.SelectorBreadcrumbStyle().Render(searchPrompt + s.nav.Search + "▏"))
	}
	sb.WriteString("\n\n")

	boxWidth := kit.CalculateToolBoxWidth(s.width)
	maxDescLen := max(boxWidth-30, 20)

	if len(s.filteredTools) == 0 {
		sb.WriteString(kit.SelectorHintStyle().Render("  No tools match the filter"))
		sb.WriteString("\n")
	} else {
		startIdx, endIdx := s.nav.VisibleRange()

		if startIdx > 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			t := s.filteredTools[i]

			var statusIcon string
			var statusStyle lipgloss.Style
			if t.Enabled {
				statusIcon = "●"
				statusStyle = kit.SelectorStatusConnected()
			} else {
				statusIcon = "○"
				statusStyle = kit.SelectorStatusNone()
			}

			desc := t.Description
			if idx := strings.Index(desc, "\n"); idx != -1 {
				desc = desc[:idx]
			}
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}

			descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s  %s",
				statusStyle.Render(statusIcon),
				t.Name,
				descStyle.Render(desc),
			)

			if i == s.nav.Selected {
				sb.WriteString(kit.SelectorSelectedStyle().Render("> " + line))
			} else {
				sb.WriteString(kit.SelectorItemStyle().Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredTools) {
			sb.WriteString(kit.SelectorHintStyle().Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(kit.SelectorHintStyle().Render("↑/↓ navigate · Enter toggle · Tab level · Esc cancel"))

	// Wrap in border
	content := sb.String()
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

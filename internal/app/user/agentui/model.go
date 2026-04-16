// Package agent provides the agent selector feature.
package agentui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// item represents an agent in the kit.
type item struct {
	Name           string
	Description    string
	Model          string // inherit/sonnet/opus/haiku
	PermissionMode string // default/dontAsk/plan/bypass
	Tools          string // Tool list as string
	IsCustom       bool   // Whether this is a custom agent
	PluginName     string // Plugin name if from a plugin (e.g., "code-simplifier")
	Enabled        bool   // Current enabled state
}

// Model holds the state for the agent selector.
type Model struct {
	registry       *subagent.Registry
	active         bool
	agents         []item
	filteredAgents []item
	nav            kit.ListNav
	width          int
	height         int
	saveLevel      kit.SaveLevel
}

// ToggleMsg is sent when an agent's enabled state is toggled.
type ToggleMsg struct {
	AgentName string
	Enabled   bool
}

// New creates a new agent selector Model.
func New(reg *subagent.Registry) Model {
	return Model{
		registry: reg,
		active:   false,
		agents:   []item{},
		nav:      kit.ListNav{MaxVisible: 10},
	}
}

// EnterSelect enters agent selection mode.
func (s *Model) EnterSelect(width, height int) error {
	allConfigs := s.registry.ListConfigs()
	disabledAgents := s.registry.GetDisabledAt(s.saveLevel == kit.SaveLevelUser)

	s.agents = make([]item, 0, len(allConfigs))
	for _, cfg := range allConfigs {
		lowerName := strings.ToLower(cfg.Name)

		var pluginName string
		if idx := strings.Index(cfg.Name, ":"); idx > 0 {
			pluginName = cfg.Name[:idx]
		}

		s.agents = append(s.agents, item{
			Name:           cfg.Name,
			Description:    cfg.Description,
			Model:          cfg.Model,
			PermissionMode: formatPermissionMode(cfg.PermissionMode),
			Tools:          formatTools(cfg.Tools),
			IsCustom:       cfg.SourceFile != "" && pluginName == "",
			PluginName:     pluginName,
			Enabled:        !disabledAgents[lowerName],
		})
	}

	s.active = true
	s.width = width
	s.height = height
	s.filteredAgents = s.agents
	s.nav.Reset()
	s.nav.Total = len(s.filteredAgents)

	return nil
}

// formatPermissionMode converts PermissionMode to display string.
func formatPermissionMode(mode subagent.PermissionMode) string {
	switch mode {
	case subagent.PermissionPlan:
		return "plan"
	case subagent.PermissionAcceptEdits:
		return "acceptEdits"
	case subagent.PermissionDontAsk:
		return "dontAsk"
	case subagent.PermissionBypassPermissions:
		return "bypass"
	case subagent.PermissionAuto:
		return "auto"
	default:
		return "default"
	}
}

// formatTools formats a tool list for display.
func formatTools(tools subagent.ToolList) string {
	if tools == nil {
		return "all tools"
	}
	if len(tools) == 0 {
		return "none"
	}
	return strings.Join([]string(tools), ", ")
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the kit.
func (s *Model) Cancel() {
	s.active = false
	s.agents = []item{}
	s.filteredAgents = []item{}
	s.nav.Reset()
	s.nav.Total = 0
}

// updateFilter filters agents based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.nav.Search == "" {
		s.filteredAgents = s.agents
	} else {
		query := strings.ToLower(s.nav.Search)
		s.filteredAgents = make([]item, 0)
		for _, a := range s.agents {
			if kit.FuzzyMatch(strings.ToLower(a.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(a.Description), query) {
				s.filteredAgents = append(s.filteredAgents, a)
			}
		}
	}
	s.nav.ResetCursor()
	s.nav.Total = len(s.filteredAgents)
}

// reloadAgentStates reloads the enabled/disabled states from the current save level.
func (s *Model) reloadAgentStates() {
	disabledAgents := s.registry.GetDisabledAt(s.saveLevel == kit.SaveLevelUser)

	for i := range s.agents {
		lowerName := strings.ToLower(s.agents[i].Name)
		s.agents[i].Enabled = !disabledAgents[lowerName]
	}
	for i := range s.filteredAgents {
		lowerName := strings.ToLower(s.filteredAgents[i].Name)
		s.filteredAgents[i].Enabled = !disabledAgents[lowerName]
	}
}

// Toggle toggles the enabled state of the currently selected agent.
func (s *Model) Toggle() tea.Cmd {
	if len(s.filteredAgents) == 0 || s.nav.Selected >= len(s.filteredAgents) {
		return nil
	}

	selected := &s.filteredAgents[s.nav.Selected]
	selected.Enabled = !selected.Enabled

	for i := range s.agents {
		if s.agents[i].Name == selected.Name {
			s.agents[i].Enabled = selected.Enabled
			break
		}
	}

	_ = s.registry.SetEnabled(
		selected.Name,
		selected.Enabled,
		s.saveLevel == kit.SaveLevelUser,
	)

	return func() tea.Msg {
		return ToggleMsg{
			AgentName: selected.Name,
			Enabled:   selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed.
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Tab toggles save level
	if key.Type == tea.KeyTab {
		if s.saveLevel == kit.SaveLevelProject {
			s.saveLevel = kit.SaveLevelUser
		} else {
			s.saveLevel = kit.SaveLevelProject
		}
		s.reloadAgentStates()
		return nil
	}

	// Enter toggles the selected agent
	if key.Type == tea.KeyEnter {
		return s.Toggle()
	}

	// Delegate navigation and search to ListNav
	searchChanged, consumed := s.nav.HandleKey(key)
	if searchChanged {
		s.updateFilter()
	}
	if consumed {
		return nil
	}

	// Esc with empty search — dismiss
	if key.Type == tea.KeyEsc {
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
	}

	return nil
}

// calculateBoxWidth returns the constrained box width for agent kit.
func calculateBoxWidth(screenWidth int) int {
	boxWidth := screenWidth * 85 / 100
	return max(70, boxWidth)
}

// Render renders the agent kit.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Agents (%d/%d)  %s", len(s.filteredAgents), len(s.agents), levelIndicator)
	sb.WriteString(kit.SelectorTitleStyle().Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "\U0001f50d "
	if s.nav.Search == "" {
		sb.WriteString(kit.SelectorHintStyle().Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(kit.SelectorBreadcrumbStyle().Render(searchPrompt + s.nav.Search + "\u258f"))
	}
	sb.WriteString("\n\n")

	boxWidth := calculateBoxWidth(s.width)

	if len(s.filteredAgents) == 0 {
		sb.WriteString(kit.SelectorHintStyle().Render("  No agents match the filter"))
		sb.WriteString("\n")
	} else {
		startIdx, endIdx := s.nav.VisibleRange()

		if startIdx > 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  \u2191 more above"))
			sb.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			a := s.filteredAgents[i]

			var statusIcon string
			var statusStyle lipgloss.Style
			if a.Enabled {
				statusIcon = "\u25cf"
				statusStyle = kit.SelectorStatusConnected()
			} else {
				statusIcon = "\u25cb"
				statusStyle = kit.SelectorStatusNone()
			}

			name := a.Name
			if len(name) > 15 {
				name = name[:12] + "..."
			}

			model := a.Model
			if len(model) > 7 {
				model = model[:7]
			}

			mode := a.PermissionMode
			if len(mode) > 8 {
				mode = mode[:8]
			}

			toolsWidth := boxWidth - 54
			if toolsWidth < 10 {
				toolsWidth = 10
			}

			tools := a.Tools
			if len(tools) > toolsWidth {
				tools = tools[:toolsWidth-3] + "..."
			}

			sourceTag := ""
			if a.PluginName != "" {
				sourceTag = " [Plugin: " + a.PluginName + "]"
			} else if a.IsCustom {
				sourceTag = " [Custom]"
			}

			descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s %-7s %-8s %s%s",
				statusStyle.Render(statusIcon),
				name,
				model,
				mode,
				descStyle.Render(tools),
				sourceTag,
			)

			if i == s.nav.Selected {
				sb.WriteString(kit.SelectorSelectedStyle().Render("> " + line))
			} else {
				sb.WriteString(kit.SelectorItemStyle().Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredAgents) {
			sb.WriteString(kit.SelectorHintStyle().Render("  \u2193 more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(kit.SelectorHintStyle().Render("\u2191/\u2193 navigate \u00b7 Enter toggle \u00b7 Tab level \u00b7 Esc cancel"))

	content := sb.String()
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)

	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

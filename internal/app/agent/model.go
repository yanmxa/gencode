// Package agent provides the agent selector feature.
package agent

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreagent "github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Item represents an agent in the selector.
type Item struct {
	Name           string
	Description    string
	Model          string // inherit/sonnet/opus/haiku
	PermissionMode string // default/dontAsk/plan/bypass
	Tools          string // Tool list as string
	IsCustom       bool   // Whether this is a custom agent
	PluginName     string // Plugin name if from a plugin (e.g., "code-simplifier")
	Enabled        bool   // Current enabled state
}

// SaveLevel represents where to save agent settings.
type SaveLevel int

const (
	SaveLevelProject SaveLevel = iota // Save to .gen/agents.json
	SaveLevelUser                     // Save to ~/.gen/agents.json
)

// String returns the display name for the save level.
func (l SaveLevel) String() string {
	switch l {
	case SaveLevelUser:
		return "User"
	default:
		return "Project"
	}
}

// Model holds the state for the agent selector.
type Model struct {
	active         bool
	agents         []Item
	filteredAgents []Item
	selectedIdx    int
	width          int
	height         int
	searchQuery    string
	scrollOffset   int
	maxVisible     int
	saveLevel      SaveLevel
}

// ToggleMsg is sent when an agent's enabled state is toggled.
type ToggleMsg struct {
	AgentName string
	Enabled   bool
}


// New creates a new agent selector Model.
func New() Model {
	return Model{
		active:     false,
		agents:     []Item{},
		maxVisible: 10,
	}
}

// EnterSelect enters agent selection mode.
func (s *Model) EnterSelect(width, height int) error {
	// Get all agent configs from registry
	allConfigs := coreagent.DefaultRegistry.ListConfigs()

	// Get disabled agents for the current level
	disabledAgents := coreagent.DefaultRegistry.GetDisabledAt(s.saveLevel == SaveLevelUser)

	s.agents = make([]Item, 0, len(allConfigs))
	for _, cfg := range allConfigs {
		lowerName := strings.ToLower(cfg.Name)

		// Detect plugin agents by namespace prefix (e.g., "plugin-name:agent-name")
		var pluginName string
		if idx := strings.Index(cfg.Name, ":"); idx > 0 {
			pluginName = cfg.Name[:idx]
		}

		s.agents = append(s.agents, Item{
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
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
	s.width = width
	s.height = height
	s.filteredAgents = s.agents

	return nil
}

// formatPermissionMode converts PermissionMode to display string.
func formatPermissionMode(mode coreagent.PermissionMode) string {
	switch mode {
	case coreagent.PermissionPlan:
		return "plan"
	case coreagent.PermissionAcceptEdits:
		return "acceptEdits"
	case coreagent.PermissionDontAsk:
		return "dontAsk"
	case coreagent.PermissionBypassPermissions:
		return "bypass"
	case coreagent.PermissionAuto:
		return "auto"
	default:
		return "default"
	}
}

// formatTools formats a tool list for display.
func formatTools(tools coreagent.ToolList) string {
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

// Cancel cancels the selector.
func (s *Model) Cancel() {
	s.active = false
	s.agents = []Item{}
	s.filteredAgents = []Item{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
}

// MoveUp moves the selection up.
func (s *Model) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down.
func (s *Model) MoveDown() {
	if s.selectedIdx < len(s.filteredAgents)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible.
func (s *Model) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters agents based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredAgents = s.agents
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredAgents = make([]Item, 0)
		for _, a := range s.agents {
			if shared.FuzzyMatch(strings.ToLower(a.Name), query) ||
				shared.FuzzyMatch(strings.ToLower(a.Description), query) {
				s.filteredAgents = append(s.filteredAgents, a)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// reloadAgentStates reloads the enabled/disabled states from the current save level.
func (s *Model) reloadAgentStates() {
	disabledAgents := coreagent.DefaultRegistry.GetDisabledAt(s.saveLevel == SaveLevelUser)

	// Update agent enabled states
	for i := range s.agents {
		lowerName := strings.ToLower(s.agents[i].Name)
		s.agents[i].Enabled = !disabledAgents[lowerName]
	}

	// Update filtered agents
	for i := range s.filteredAgents {
		lowerName := strings.ToLower(s.filteredAgents[i].Name)
		s.filteredAgents[i].Enabled = !disabledAgents[lowerName]
	}
}

// Toggle toggles the enabled state of the currently selected agent.
func (s *Model) Toggle() tea.Cmd {
	if len(s.filteredAgents) == 0 || s.selectedIdx >= len(s.filteredAgents) {
		return nil
	}

	selected := &s.filteredAgents[s.selectedIdx]
	selected.Enabled = !selected.Enabled

	// Update the source agents list
	for i := range s.agents {
		if s.agents[i].Name == selected.Name {
			s.agents[i].Enabled = selected.Enabled
			break
		}
	}

	// Save to registry (project or user level based on saveLevel)
	_ = coreagent.DefaultRegistry.SetEnabled(
		selected.Name,
		selected.Enabled,
		s.saveLevel == SaveLevelUser,
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
		s.reloadAgentStates()
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
			return shared.DismissedMsg{}
		}
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
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

// calculateBoxWidth returns the constrained box width for agent selector.
func calculateBoxWidth(screenWidth int) int {
	boxWidth := screenWidth * 85 / 100
	return max(70, boxWidth)
}

// Render renders the agent selector.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Agents (%d/%d)  %s", len(s.filteredAgents), len(s.agents), levelIndicator)
	sb.WriteString(shared.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "\U0001f50d "
	if s.searchQuery == "" {
		sb.WriteString(shared.SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(shared.SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "\u258f"))
	}
	sb.WriteString("\n\n")

	// Calculate box width
	boxWidth := calculateBoxWidth(s.width)

	// Handle empty results
	if len(s.filteredAgents) == 0 {
		sb.WriteString(shared.SelectorHintStyle.Render("  No agents match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredAgents))

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(shared.SelectorHintStyle.Render("  \u2191 more above"))
			sb.WriteString("\n")
		}

		// Render visible agents
		for i := s.scrollOffset; i < endIdx; i++ {
			a := s.filteredAgents[i]

			// Status icon: filled enabled (green), empty disabled (gray)
			var statusIcon string
			var statusStyle lipgloss.Style
			if a.Enabled {
				statusIcon = "\u25cf"
				statusStyle = shared.SelectorStatusConnected
			} else {
				statusIcon = "\u25cb"
				statusStyle = shared.SelectorStatusNone
			}

			// Format agent info
			// Name (15 chars) | Model (7 chars) | Mode (8 chars) | Tools (variable) | [Custom]
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

			// Calculate remaining width for tools
			// Box - border(2) - padding(4) - prefix(2) - icon(2) - name(15) - model(7) - mode(8) - spacing(6) - custom(8)
			toolsWidth := boxWidth - 54
			if toolsWidth < 10 {
				toolsWidth = 10
			}

			tools := a.Tools
			if len(tools) > toolsWidth {
				tools = tools[:toolsWidth-3] + "..."
			}

			// Source indicator (Plugin or Custom)
			sourceTag := ""
			if a.PluginName != "" {
				sourceTag = " [Plugin: " + a.PluginName + "]"
			} else if a.IsCustom {
				sourceTag = " [Custom]"
			}

			descStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s %-7s %-8s %s%s",
				statusStyle.Render(statusIcon),
				name,
				model,
				mode,
				descStyle.Render(tools),
				sourceTag,
			)

			if i == s.selectedIdx {
				sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredAgents) {
			sb.WriteString(shared.SelectorHintStyle.Render("  \u2193 more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(shared.SelectorHintStyle.Render("\u2191/\u2193 navigate \u00b7 Enter toggle \u00b7 Tab level \u00b7 Esc cancel"))

	// Wrap in border
	content := sb.String()
	box := shared.SelectorBorderStyle.Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/agent"
)

// AgentItem represents an agent in the selector
type AgentItem struct {
	Name           string
	Description    string
	Model          string // inherit/sonnet/opus/haiku
	PermissionMode string // default/acceptEdits/dontAsk/plan
	Tools          string // Tool list as string
	IsCustom       bool   // Whether this is a custom agent
	Enabled        bool   // Current enabled state
}

// AgentSaveLevel represents where to save agent settings
type AgentSaveLevel int

const (
	AgentSaveLevelProject AgentSaveLevel = iota // Save to .gen/agents.json
	AgentSaveLevelUser                          // Save to ~/.gen/agents.json
)

// String returns the display name for the save level
func (l AgentSaveLevel) String() string {
	switch l {
	case AgentSaveLevelUser:
		return "User"
	default:
		return "Project"
	}
}

// AgentSelectorState holds state for the agent selector
type AgentSelectorState struct {
	active         bool
	agents         []AgentItem
	filteredAgents []AgentItem
	selectedIdx    int
	width          int
	height         int
	searchQuery    string
	scrollOffset   int
	maxVisible     int
	saveLevel      AgentSaveLevel
}

// AgentToggleMsg is sent when an agent's enabled state is toggled
type AgentToggleMsg struct {
	AgentName string
	Enabled   bool
}

// AgentSelectorCancelledMsg is sent when the agent selector is cancelled
type AgentSelectorCancelledMsg struct{}

// NewAgentSelectorState creates a new AgentSelectorState
func NewAgentSelectorState() AgentSelectorState {
	return AgentSelectorState{
		active:     false,
		agents:     []AgentItem{},
		maxVisible: 10,
	}
}

// EnterAgentSelect enters agent selection mode
func (s *AgentSelectorState) EnterAgentSelect(width, height int) error {
	// Get all agent configs from registry
	allConfigs := agent.DefaultRegistry.ListConfigs()

	// Get disabled agents for the current level
	disabledAgents := agent.DefaultRegistry.GetDisabledAt(s.saveLevel == AgentSaveLevelUser)

	s.agents = make([]AgentItem, 0, len(allConfigs))
	for _, cfg := range allConfigs {
		lowerName := strings.ToLower(cfg.Name)
		s.agents = append(s.agents, AgentItem{
			Name:           cfg.Name,
			Description:    cfg.Description,
			Model:          cfg.Model,
			PermissionMode: formatPermissionMode(cfg.PermissionMode),
			Tools:          formatToolsAccess(cfg.Tools),
			IsCustom:       cfg.SourceFile != "",
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

// formatPermissionMode converts PermissionMode to display string
func formatPermissionMode(mode agent.PermissionMode) string {
	switch mode {
	case agent.PermissionPlan:
		return "plan"
	case agent.PermissionAcceptEdits:
		return "accept"
	case agent.PermissionDontAsk:
		return "dontAsk"
	default:
		return "default"
	}
}

// formatToolsAccess formats tool access config for display
func formatToolsAccess(access agent.ToolAccess) string {
	switch access.Mode {
	case agent.ToolAccessAllowlist:
		if len(access.Allow) == 0 {
			return "none"
		}
		return strings.Join(access.Allow, ", ")
	case agent.ToolAccessDenylist:
		if len(access.Deny) == 0 {
			return "all tools"
		}
		return fmt.Sprintf("all except %s", strings.Join(access.Deny, ", "))
	default:
		return "default"
	}
}

// IsActive returns whether the selector is active
func (s *AgentSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *AgentSelectorState) Cancel() {
	s.active = false
	s.agents = []AgentItem{}
	s.filteredAgents = []AgentItem{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
}

// MoveUp moves the selection up
func (s *AgentSelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *AgentSelectorState) MoveDown() {
	if s.selectedIdx < len(s.filteredAgents)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *AgentSelectorState) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters agents based on search query (fuzzy match)
func (s *AgentSelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filteredAgents = s.agents
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredAgents = make([]AgentItem, 0)
		for _, a := range s.agents {
			if fuzzyMatch(strings.ToLower(a.Name), query) ||
				fuzzyMatch(strings.ToLower(a.Description), query) {
				s.filteredAgents = append(s.filteredAgents, a)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// reloadAgentStates reloads the enabled/disabled states from the current save level
func (s *AgentSelectorState) reloadAgentStates() {
	disabledAgents := agent.DefaultRegistry.GetDisabledAt(s.saveLevel == AgentSaveLevelUser)

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

// Toggle toggles the enabled state of the currently selected agent
func (s *AgentSelectorState) Toggle() tea.Cmd {
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
	_ = agent.DefaultRegistry.SetEnabled(
		selected.Name,
		selected.Enabled,
		s.saveLevel == AgentSaveLevelUser,
	)

	return func() tea.Msg {
		return AgentToggleMsg{
			AgentName: selected.Name,
			Enabled:   selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *AgentSelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyTab:
		// Toggle save level between project and user
		if s.saveLevel == AgentSaveLevelProject {
			s.saveLevel = AgentSaveLevelUser
		} else {
			s.saveLevel = AgentSaveLevelProject
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
			return AgentSelectorCancelledMsg{}
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

// calculateAgentBoxWidth returns the constrained box width for agent selector
func calculateAgentBoxWidth(screenWidth int) int {
	boxWidth := screenWidth * 85 / 100
	return max(70, boxWidth)
}

// Render renders the agent selector
func (s *AgentSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Agents (%d/%d)  %s", len(s.filteredAgents), len(s.agents), levelIndicator)
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

	// Calculate box width
	boxWidth := calculateAgentBoxWidth(s.width)

	// Handle empty results
	if len(s.filteredAgents) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No agents match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredAgents))

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(selectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		// Render visible agents
		for i := s.scrollOffset; i < endIdx; i++ {
			a := s.filteredAgents[i]

			// Status icon: ● enabled (green), ○ disabled (gray)
			var statusIcon string
			var statusStyle lipgloss.Style
			if a.Enabled {
				statusIcon = "●"
				statusStyle = selectorStatusConnected
			} else {
				statusIcon = "○"
				statusStyle = selectorStatusNone
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

			// Custom indicator
			customTag := ""
			if a.IsCustom {
				customTag = " [Custom]"
			}

			descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s %-7s %-8s %s%s",
				statusStyle.Render(statusIcon),
				name,
				model,
				mode,
				descStyle.Render(tools),
				customTag,
			)

			if i == s.selectedIdx {
				sb.WriteString(selectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(selectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredAgents) {
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

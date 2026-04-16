// Package skill provides the skill selector feature.
package skillui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreskill "github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// item represents a skill in the kit.
type item struct {
	Name        string // Base name
	Namespace   string // Optional namespace
	Description string
	Hint        string // argument-hint
	State       coreskill.SkillState
	Scope       coreskill.SkillScope
}

// FullName returns the namespaced skill name (namespace:name or just name).
func (s *item) FullName() string {
	if s.Namespace != "" {
		return s.Namespace + ":" + s.Name
	}
	return s.Name
}

// Model holds state for the skill selector.
type Model struct {
	registry       *coreskill.Registry
	active         bool
	skills         []item
	filteredSkills []item
	nav            kit.ListNav
	width          int
	height         int
	saveLevel      kit.SaveLevel
}

// CycleMsg is sent when a skill's state is cycled.
type CycleMsg struct {
	SkillName string
	NewState  coreskill.SkillState
}

// InvokeMsg is sent when a skill is invoked from the kit.
type InvokeMsg struct {
	SkillName string
}

// New creates a new skill selector Model.
func New(reg *coreskill.Registry) Model {
	return Model{
		registry: reg,
		active:   false,
		skills:   []item{},
		nav:      kit.ListNav{MaxVisible: 10},
	}
}

// EnterSelect enters skill selection mode.
func (s *Model) EnterSelect(width, height int) error {
	if s.registry == nil {
		return fmt.Errorf("skill registry not initialized")
	}

	allSkills := s.registry.List()
	levelStates := s.registry.GetStatesAt(s.saveLevel == kit.SaveLevelUser)

	s.skills = make([]item, 0, len(allSkills))
	for _, sk := range allSkills {
		state := sk.State
		if levelState, ok := levelStates[sk.FullName()]; ok {
			state = levelState
		}
		s.skills = append(s.skills, item{
			Name:        sk.Name,
			Namespace:   sk.Namespace,
			Description: sk.Description,
			Hint:        sk.ArgumentHint,
			State:       state,
			Scope:       sk.Scope,
		})
	}

	s.active = true
	s.width = width
	s.height = height
	s.filteredSkills = s.skills
	s.nav.Reset()
	s.nav.Total = len(s.filteredSkills)

	return nil
}

// reloadSkillStates reloads skill states from the current save level.
func (s *Model) reloadSkillStates() {
	if s.registry == nil {
		return
	}

	levelStates := s.registry.GetStatesAt(s.saveLevel == kit.SaveLevelUser)

	for i := range s.skills {
		fullName := s.skills[i].FullName()
		if state, ok := levelStates[fullName]; ok {
			s.skills[i].State = state
		} else {
			s.skills[i].State = coreskill.StateEnable
		}
	}
	s.updateFilter()
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the kit.
func (s *Model) Cancel() {
	s.active = false
	s.skills = []item{}
	s.filteredSkills = []item{}
	s.nav.Reset()
	s.nav.Total = 0
}

// updateFilter filters skills based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.nav.Search == "" {
		s.filteredSkills = s.skills
	} else {
		query := strings.ToLower(s.nav.Search)
		s.filteredSkills = make([]item, 0)
		for _, sk := range s.skills {
			if kit.FuzzyMatch(strings.ToLower(sk.FullName()), query) ||
				kit.FuzzyMatch(strings.ToLower(sk.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(sk.Description), query) {
				s.filteredSkills = append(s.filteredSkills, sk)
			}
		}
	}
	s.nav.ResetCursor()
	s.nav.Total = len(s.filteredSkills)
}

// CycleState cycles the state of the currently selected skill.
func (s *Model) CycleState() tea.Cmd {
	if len(s.filteredSkills) == 0 || s.nav.Selected >= len(s.filteredSkills) {
		return nil
	}

	selected := &s.filteredSkills[s.nav.Selected]
	newState := selected.State.NextState()
	selected.State = newState

	fullName := selected.FullName()

	for i := range s.skills {
		if s.skills[i].FullName() == fullName {
			s.skills[i].State = newState
			break
		}
	}

	if s.registry != nil {
		_ = s.registry.SetState(fullName, newState, s.saveLevel == kit.SaveLevelUser)
	}

	return func() tea.Msg {
		return CycleMsg{
			SkillName: fullName,
			NewState:  newState,
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
		s.reloadSkillStates()
		return nil
	}

	// Enter cycles the state
	if key.Type == tea.KeyEnter {
		return s.CycleState()
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

// calculateSkillBoxWidth returns the constrained box width for skill kit.
func calculateSkillBoxWidth(screenWidth int) int {
	boxWidth := screenWidth * 80 / 100
	return max(60, boxWidth)
}

// Render renders the skill kit.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	boxWidth := calculateSkillBoxWidth(s.width)

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Skills (%d/%d)  %s", len(s.filteredSkills), len(s.skills), levelIndicator)
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

	if len(s.filteredSkills) == 0 {
		sb.WriteString(kit.SelectorHintStyle().Render("  No skills match the filter"))
		sb.WriteString("\n")
	} else {
		startIdx, endIdx := s.nav.VisibleRange()

		// Content width is box width minus border (2) and padding (4)
		contentWidth := boxWidth - 6

		// Calculate max name width from visible skills for proper alignment
		maxNameWidth := 0
		for i := startIdx; i < endIdx; i++ {
			nameLen := len(s.filteredSkills[i].FullName())
			if nameLen > maxNameWidth {
				maxNameWidth = nameLen
			}
		}
		maxNameWidth += 5 // space for [P] indicator
		maxNameWidth = max(15, min(30, maxNameWidth))

		if startIdx > 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  \u2191 more above"))
			sb.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			sk := s.filteredSkills[i]

			var statusIcon string
			var statusStyle lipgloss.Style
			switch sk.State {
			case coreskill.StateActive:
				statusIcon = "\u25cf"
				statusStyle = kit.SelectorStatusConnected()
			case coreskill.StateEnable:
				statusIcon = "\u25d0"
				statusStyle = lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning)
			default:
				statusIcon = "\u25cb"
				statusStyle = kit.SelectorStatusNone()
			}

			displayName := sk.FullName()

			scopeIndicator := ""
			if sk.Scope == coreskill.ScopeProject || sk.Scope == coreskill.ScopeClaudeProject || sk.Scope == coreskill.ScopeProjectPlugin {
				scopeIndicator = "[P]"
			}

			nameWithScope := displayName
			if scopeIndicator != "" {
				nameWithScope = displayName + " " + scopeIndicator
			}
			paddedName := nameWithScope
			if len(paddedName) < maxNameWidth {
				paddedName = paddedName + strings.Repeat(" ", maxNameWidth-len(paddedName))
			} else if len(paddedName) > maxNameWidth {
				paddedName = paddedName[:maxNameWidth-3] + "..."
			}

			usedWidth := 6 + maxNameWidth
			descMaxLen := contentWidth - usedWidth
			if descMaxLen < 15 {
				descMaxLen = 15
			}

			desc := sk.Description
			if len(desc) > descMaxLen {
				desc = desc[:descMaxLen-3] + "..."
			}

			descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)

			line := fmt.Sprintf("%s %-*s  %s",
				statusStyle.Render(statusIcon),
				maxNameWidth,
				paddedName,
				descStyle.Render(desc),
			)

			if i == s.nav.Selected {
				sb.WriteString("> " + line)
			} else {
				sb.WriteString("  " + line)
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredSkills) {
			sb.WriteString(kit.SelectorHintStyle().Render("  \u2193 more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(kit.SelectorHintStyle().Render("\u2191/\u2193 navigate \u00b7 Tab level \u00b7 Enter toggle \u00b7 Esc close"))

	content := sb.String()
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)

	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

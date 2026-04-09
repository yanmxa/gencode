// Package skill provides the skill selector feature.
package skillui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreskill "github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/ui/selector"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Item represents a skill in the selector.
type Item struct {
	Name        string // Base name
	Namespace   string // Optional namespace
	Description string
	Hint        string // argument-hint
	State       coreskill.SkillState
	Scope       coreskill.SkillScope
}

// FullName returns the namespaced skill name (namespace:name or just name).
func (s *Item) FullName() string {
	if s.Namespace != "" {
		return s.Namespace + ":" + s.Name
	}
	return s.Name
}

// SaveLevel represents where to save skill settings.
type SaveLevel int

const (
	SaveLevelProject SaveLevel = iota // Save to .gen/skills.json
	SaveLevelUser                     // Save to ~/.gen/skills.json
)

// String returns the display name for the save level.
func (l SaveLevel) String() string {
	if l == SaveLevelUser {
		return "User"
	}
	return "Project"
}

// Model holds state for the skill selector.
type Model struct {
	active         bool
	skills         []Item
	filteredSkills []Item
	selectedIdx    int
	width          int
	height         int
	searchQuery    string
	scrollOffset   int
	maxVisible     int
	saveLevel      SaveLevel // Where to save settings (project or user)
}

// CycleMsg is sent when a skill's state is cycled.
type CycleMsg struct {
	SkillName string
	NewState  coreskill.SkillState
}

// InvokeMsg is sent when a skill is invoked from the selector.
type InvokeMsg struct {
	SkillName string
}

// New creates a new skill selector Model.
func New() Model {
	return Model{
		active:     false,
		skills:     []Item{},
		maxVisible: 10,
	}
}

// EnterSelect enters skill selection mode.
func (s *Model) EnterSelect(width, height int) error {
	if coreskill.DefaultRegistry == nil {
		return fmt.Errorf("skill registry not initialized")
	}

	allSkills := coreskill.DefaultRegistry.List()

	// Get states from current level
	levelStates := coreskill.DefaultRegistry.GetStatesAt(s.saveLevel == SaveLevelUser)

	s.skills = make([]Item, 0, len(allSkills))
	for _, sk := range allSkills {
		// Use level-specific state if available, otherwise use merged state
		state := sk.State
		if levelState, ok := levelStates[sk.FullName()]; ok {
			state = levelState
		}
		s.skills = append(s.skills, Item{
			Name:        sk.Name,
			Namespace:   sk.Namespace,
			Description: sk.Description,
			Hint:        sk.ArgumentHint,
			State:       state,
			Scope:       sk.Scope,
		})
	}

	s.active = true
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
	s.width = width
	s.height = height
	s.filteredSkills = s.skills

	return nil
}

// reloadSkillStates reloads skill states from the current save level.
func (s *Model) reloadSkillStates() {
	if coreskill.DefaultRegistry == nil {
		return
	}

	levelStates := coreskill.DefaultRegistry.GetStatesAt(s.saveLevel == SaveLevelUser)

	for i := range s.skills {
		fullName := s.skills[i].FullName()
		if state, ok := levelStates[fullName]; ok {
			s.skills[i].State = state
		} else {
			s.skills[i].State = coreskill.StateEnable // Default
		}
	}
	s.updateFilter()
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector.
func (s *Model) Cancel() {
	s.active = false
	s.skills = []Item{}
	s.filteredSkills = []Item{}
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
	if s.selectedIdx < len(s.filteredSkills)-1 {
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

// updateFilter filters skills based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredSkills = s.skills
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredSkills = make([]Item, 0)
		for _, sk := range s.skills {
			// Match against full name (namespace:name), name, or description
			if selector.FuzzyMatch(strings.ToLower(sk.FullName()), query) ||
				selector.FuzzyMatch(strings.ToLower(sk.Name), query) ||
				selector.FuzzyMatch(strings.ToLower(sk.Description), query) {
				s.filteredSkills = append(s.filteredSkills, sk)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// CycleState cycles the state of the currently selected skill.
func (s *Model) CycleState() tea.Cmd {
	if len(s.filteredSkills) == 0 || s.selectedIdx >= len(s.filteredSkills) {
		return nil
	}

	selected := &s.filteredSkills[s.selectedIdx]
	newState := selected.State.NextState()
	selected.State = newState

	// Use FullName for comparison and persistence
	fullName := selected.FullName()

	// Update the source skills list
	for i := range s.skills {
		if s.skills[i].FullName() == fullName {
			s.skills[i].State = newState
			break
		}
	}

	// Persist via registry (using FullName) to the current save level
	if coreskill.DefaultRegistry != nil {
		_ = coreskill.DefaultRegistry.SetState(fullName, newState, s.saveLevel == SaveLevelUser)
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
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyTab:
		// Tab toggles save level between project and user
		if s.saveLevel == SaveLevelProject {
			s.saveLevel = SaveLevelUser
		} else {
			s.saveLevel = SaveLevelProject
		}
		// Reload skill states from the new level
		s.reloadSkillStates()
		return nil
	case tea.KeyEnter:
		// Enter cycles the state
		return s.CycleState()
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

// calculateSkillBoxWidth returns the constrained box width for skill selector.
func calculateSkillBoxWidth(screenWidth int) int {
	// Use 80% of screen width
	boxWidth := screenWidth * 80 / 100
	return max(60, boxWidth)
}

// Render renders the skill selector.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Calculate box width for dynamic description length
	boxWidth := calculateSkillBoxWidth(s.width)

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Skills (%d/%d)  %s", len(s.filteredSkills), len(s.skills), levelIndicator)
	sb.WriteString(selector.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "\U0001f50d "
	if s.searchQuery == "" {
		sb.WriteString(selector.SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(selector.SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "\u258f"))
	}
	sb.WriteString("\n\n")

	// Handle empty results
	if len(s.filteredSkills) == 0 {
		sb.WriteString(selector.SelectorHintStyle.Render("  No skills match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredSkills))

		// Content width is box width minus border (2) and padding (4)
		contentWidth := boxWidth - 6

		// Calculate max name width from visible skills for proper alignment
		maxNameWidth := 0
		for i := s.scrollOffset; i < endIdx; i++ {
			nameLen := len(s.filteredSkills[i].FullName())
			if nameLen > maxNameWidth {
				maxNameWidth = nameLen
			}
		}
		// Add space for [P] indicator
		maxNameWidth += 5
		// Cap at reasonable width
		if maxNameWidth > 30 {
			maxNameWidth = 30
		}
		if maxNameWidth < 15 {
			maxNameWidth = 15
		}

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(selector.SelectorHintStyle.Render("  \u2191 more above"))
			sb.WriteString("\n")
		}

		// Render visible skills
		for i := s.scrollOffset; i < endIdx; i++ {
			sk := s.filteredSkills[i]

			// Status icon: filled active (green), half enabled (yellow), empty disabled (gray)
			var statusIcon string
			var statusStyle lipgloss.Style
			switch sk.State {
			case coreskill.StateActive:
				statusIcon = "\u25cf"
				statusStyle = selector.SelectorStatusConnected
			case coreskill.StateEnable:
				statusIcon = "\u25d0"
				statusStyle = lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
			default:
				statusIcon = "\u25cb"
				statusStyle = selector.SelectorStatusNone
			}

			// Use FullName (namespace:name) for display
			displayName := sk.FullName()

			// Scope indicator for project skills
			scopeIndicator := ""
			if sk.Scope == coreskill.ScopeProject || sk.Scope == coreskill.ScopeClaudeProject || sk.Scope == coreskill.ScopeProjectPlugin {
				scopeIndicator = "[P]"
			}

			// Create padded name with scope indicator
			nameWithScope := displayName
			if scopeIndicator != "" {
				nameWithScope = displayName + " " + scopeIndicator
			}
			// Pad to max width
			paddedName := nameWithScope
			if len(paddedName) < maxNameWidth {
				paddedName = paddedName + strings.Repeat(" ", maxNameWidth-len(paddedName))
			} else if len(paddedName) > maxNameWidth {
				paddedName = paddedName[:maxNameWidth-3] + "..."
			}

			// Build description with optional hint
			// Calculate remaining space for description
			// Format: "> icon name  desc" = 2 + 2 + maxNameWidth + 2 = 6 + maxNameWidth
			usedWidth := 6 + maxNameWidth
			descMaxLen := contentWidth - usedWidth
			if descMaxLen < 15 {
				descMaxLen = 15
			}

			desc := sk.Description
			if len(desc) > descMaxLen {
				desc = desc[:descMaxLen-3] + "..."
			}

			descStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

			// Build the line without using ANSI in width calculation
			line := fmt.Sprintf("%s %-*s  %s",
				statusStyle.Render(statusIcon),
				maxNameWidth,
				paddedName,
				descStyle.Render(desc),
			)

			if i == s.selectedIdx {
				sb.WriteString("> " + line)
			} else {
				sb.WriteString("  " + line)
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredSkills) {
			sb.WriteString(selector.SelectorHintStyle.Render("  \u2193 more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selector.SelectorHintStyle.Render("\u2191/\u2193 navigate \u00b7 Tab level \u00b7 Enter toggle \u00b7 Esc close"))

	// Wrap in border
	content := sb.String()
	box := selector.SelectorBorderStyle.Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

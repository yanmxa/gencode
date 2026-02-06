package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/skill"
)

// SkillItem represents a skill in the selector.
type SkillItem struct {
	Name        string // Base name
	Namespace   string // Optional namespace
	Description string
	Hint        string // argument-hint
	State       skill.SkillState
	Scope       skill.SkillScope
}

// FullName returns the namespaced skill name (namespace:name or just name).
func (s *SkillItem) FullName() string {
	if s.Namespace != "" {
		return s.Namespace + ":" + s.Name
	}
	return s.Name
}

// SkillSaveLevel represents where to save skill settings.
type SkillSaveLevel int

const (
	SkillSaveLevelProject SkillSaveLevel = iota // Save to .gen/skills.json
	SkillSaveLevelUser                          // Save to ~/.gen/skills.json
)

// String returns the display name for the save level.
func (l SkillSaveLevel) String() string {
	if l == SkillSaveLevelUser {
		return "User"
	}
	return "Project"
}

// SkillSelectorState holds state for the skill selector.
type SkillSelectorState struct {
	active         bool
	skills         []SkillItem
	filteredSkills []SkillItem
	selectedIdx    int
	width          int
	height         int
	searchQuery    string
	scrollOffset   int
	maxVisible     int
	saveLevel      SkillSaveLevel // Where to save settings (project or user)
}

// SkillCycleMsg is sent when a skill's state is cycled.
type SkillCycleMsg struct {
	SkillName string
	NewState  skill.SkillState
}

// SkillSelectorCancelledMsg is sent when the skill selector is cancelled.
type SkillSelectorCancelledMsg struct{}

// SkillInvokeMsg is sent when a skill is invoked from the selector.
type SkillInvokeMsg struct {
	SkillName string
}

// NewSkillSelectorState creates a new SkillSelectorState.
func NewSkillSelectorState() SkillSelectorState {
	return SkillSelectorState{
		active:     false,
		skills:     []SkillItem{},
		maxVisible: 10,
	}
}

// EnterSkillSelect enters skill selection mode.
func (s *SkillSelectorState) EnterSkillSelect(width, height int) error {
	if skill.DefaultRegistry == nil {
		return fmt.Errorf("skill registry not initialized")
	}

	allSkills := skill.DefaultRegistry.List()

	// Get states from current level
	levelStates := skill.DefaultRegistry.GetStatesAt(s.saveLevel == SkillSaveLevelUser)

	s.skills = make([]SkillItem, 0, len(allSkills))
	for _, sk := range allSkills {
		// Use level-specific state if available, otherwise use merged state
		state := sk.State
		if levelState, ok := levelStates[sk.FullName()]; ok {
			state = levelState
		}
		s.skills = append(s.skills, SkillItem{
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
func (s *SkillSelectorState) reloadSkillStates() {
	if skill.DefaultRegistry == nil {
		return
	}

	levelStates := skill.DefaultRegistry.GetStatesAt(s.saveLevel == SkillSaveLevelUser)

	for i := range s.skills {
		fullName := s.skills[i].FullName()
		if state, ok := levelStates[fullName]; ok {
			s.skills[i].State = state
		} else {
			s.skills[i].State = skill.StateEnable // Default
		}
	}
	s.updateFilter()
}

// IsActive returns whether the selector is active.
func (s *SkillSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector.
func (s *SkillSelectorState) Cancel() {
	s.active = false
	s.skills = []SkillItem{}
	s.filteredSkills = []SkillItem{}
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
}

// MoveUp moves the selection up.
func (s *SkillSelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down.
func (s *SkillSelectorState) MoveDown() {
	if s.selectedIdx < len(s.filteredSkills)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible.
func (s *SkillSelectorState) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters skills based on search query (fuzzy match).
func (s *SkillSelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filteredSkills = s.skills
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredSkills = make([]SkillItem, 0)
		for _, sk := range s.skills {
			// Match against full name (namespace:name), name, or description
			if fuzzyMatch(strings.ToLower(sk.FullName()), query) ||
				fuzzyMatch(strings.ToLower(sk.Name), query) ||
				fuzzyMatch(strings.ToLower(sk.Description), query) {
				s.filteredSkills = append(s.filteredSkills, sk)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// CycleState cycles the state of the currently selected skill.
func (s *SkillSelectorState) CycleState() tea.Cmd {
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
	if skill.DefaultRegistry != nil {
		_ = skill.DefaultRegistry.SetState(fullName, newState, s.saveLevel == SkillSaveLevelUser)
	}

	return func() tea.Msg {
		return SkillCycleMsg{
			SkillName: fullName,
			NewState:  newState,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed.
func (s *SkillSelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyTab:
		// Tab toggles save level between project and user
		if s.saveLevel == SkillSaveLevelProject {
			s.saveLevel = SkillSaveLevelUser
		} else {
			s.saveLevel = SkillSaveLevelProject
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
			return SkillSelectorCancelledMsg{}
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
func (s *SkillSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Calculate box width for dynamic description length
	boxWidth := calculateSkillBoxWidth(s.width)

	// Title with count and save level indicator
	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Skills (%d/%d)  %s", len(s.filteredSkills), len(s.skills), levelIndicator)
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

	// Handle empty results
	if len(s.filteredSkills) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No skills match the filter"))
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
			sb.WriteString(selectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		// Render visible skills
		for i := s.scrollOffset; i < endIdx; i++ {
			sk := s.filteredSkills[i]

			// Status icon: ● active (green), ◐ enabled (yellow), ○ disabled (gray)
			var statusIcon string
			var statusStyle lipgloss.Style
			switch sk.State {
			case skill.StateActive:
				statusIcon = "●"
				statusStyle = selectorStatusConnected
			case skill.StateEnable:
				statusIcon = "◐"
				statusStyle = lipgloss.NewStyle().Foreground(CurrentTheme.Warning)
			default:
				statusIcon = "○"
				statusStyle = selectorStatusNone
			}

			// Use FullName (namespace:name) for display
			displayName := sk.FullName()

			// Scope indicator for project skills
			scopeIndicator := ""
			if sk.Scope == skill.ScopeProject || sk.Scope == skill.ScopeClaudeProject || sk.Scope == skill.ScopeProjectPlugin {
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
			// Format: "> ● name  desc" = 2 + 2 + maxNameWidth + 2 = 6 + maxNameWidth
			usedWidth := 6 + maxNameWidth
			descMaxLen := contentWidth - usedWidth
			if descMaxLen < 15 {
				descMaxLen = 15
			}

			desc := sk.Description
			if len(desc) > descMaxLen {
				desc = desc[:descMaxLen-3] + "..."
			}

			descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)

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
			sb.WriteString(selectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selectorHintStyle.Render("↑/↓ navigate · Tab level · Enter toggle · Esc close"))

	// Wrap in border
	content := sb.String()
	box := selectorBorderStyle.Width(boxWidth).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

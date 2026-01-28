package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Suggestion styles (initialized dynamically based on theme)
var (
	suggestionBoxStyle      lipgloss.Style
	selectedSuggestionStyle lipgloss.Style
	normalSuggestionStyle   lipgloss.Style
	commandNameStyle        lipgloss.Style
	commandDescStyle        lipgloss.Style
)

func init() {
	// Initialize suggestion styles based on current theme
	suggestionBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CurrentTheme.Border).
		Padding(0, 1)

	selectedSuggestionStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextBright).
		Bold(true)

	normalSuggestionStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	commandNameStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary)

	commandDescStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)
}

// SuggestionState holds the state for command suggestions
type SuggestionState struct {
	visible     bool
	suggestions []Command
	selectedIdx int
}

// NewSuggestionState creates a new SuggestionState
func NewSuggestionState() SuggestionState {
	return SuggestionState{
		visible:     false,
		suggestions: []Command{},
		selectedIdx: 0,
	}
}

// Reset resets the suggestion state
func (s *SuggestionState) Reset() {
	s.visible = false
	s.suggestions = []Command{}
	s.selectedIdx = 0
}

// UpdateSuggestions updates suggestions based on input
func (s *SuggestionState) UpdateSuggestions(input string) {
	input = strings.TrimSpace(input)

	// Only show suggestions when input starts with /
	if !strings.HasPrefix(input, "/") {
		s.visible = false
		s.suggestions = []Command{}
		s.selectedIdx = 0
		return
	}

	// Get matching commands
	s.suggestions = GetMatchingCommands(input)
	s.visible = len(s.suggestions) > 0

	// Reset selection if out of bounds
	if s.selectedIdx >= len(s.suggestions) {
		s.selectedIdx = 0
	}
}

// MoveUp moves the selection up
func (s *SuggestionState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
	}
}

// MoveDown moves the selection down
func (s *SuggestionState) MoveDown() {
	if s.selectedIdx < len(s.suggestions)-1 {
		s.selectedIdx++
	}
}

// GetSelected returns the currently selected command name, or empty string if none
func (s *SuggestionState) GetSelected() string {
	if !s.visible || len(s.suggestions) == 0 {
		return ""
	}
	if s.selectedIdx < len(s.suggestions) {
		return "/" + s.suggestions[s.selectedIdx].Name
	}
	return ""
}

// Hide hides the suggestions
func (s *SuggestionState) Hide() {
	s.visible = false
}

// IsVisible returns whether suggestions are visible
func (s *SuggestionState) IsVisible() bool {
	return s.visible && len(s.suggestions) > 0
}

// Render renders the suggestions box
func (s *SuggestionState) Render(width int) string {
	if !s.IsVisible() {
		return ""
	}

	maxItems := 5 // Show at most 5 suggestions
	items := s.suggestions
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	var lines []string
	for i, cmd := range items {
		// Format: /name - description
		cmdName := fmt.Sprintf("/%s", cmd.Name)
		desc := cmd.Description

		// Truncate description if too long
		maxDescLen := width - len(cmdName) - 8
		if maxDescLen > 0 && len(desc) > maxDescLen {
			desc = desc[:maxDescLen-3] + "..."
		}

		line := fmt.Sprintf("%s - %s", cmdName, desc)

		if i == s.selectedIdx {
			// Selected item - highlight
			lines = append(lines, selectedSuggestionStyle.Render(line))
		} else {
			// Normal item
			coloredName := commandNameStyle.Render(cmdName)
			coloredDesc := commandDescStyle.Render(" - " + desc)
			lines = append(lines, coloredName+coloredDesc)
		}
	}

	content := strings.Join(lines, "\n")
	return suggestionBoxStyle.Width(width - 4).Render(content)
}

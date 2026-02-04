package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/session"
)

// SessionSelectedMsg is sent when a session is selected
type SessionSelectedMsg struct {
	SessionID string
}

// SessionSelectorCancelledMsg is sent when the session selector is cancelled
type SessionSelectorCancelledMsg struct{}

// SessionSelectorState holds the state for the session selector
type SessionSelectorState struct {
	active       bool
	sessions     []*session.SessionMetadata
	filtered     []*session.SessionMetadata
	selectedIdx  int
	searchQuery  string
	scrollOffset int
	maxVisible   int
	width        int
	height       int
}

// NewSessionSelectorState creates a new SessionSelectorState
func NewSessionSelectorState() SessionSelectorState {
	return SessionSelectorState{
		active:     false,
		maxVisible: 15, // Show more sessions than other selectors
	}
}

// calculateSessionBoxWidth returns 70% of the screen width for the session selector
func calculateSessionBoxWidth(screenWidth int) int {
	return max(60, screenWidth*70/100)
}

// EnterSessionSelect enters session selection mode
func (s *SessionSelectorState) EnterSessionSelect(width, height int, store *session.Store) error {
	if store == nil {
		return fmt.Errorf("session store is required")
	}

	sessions, err := store.List()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found")
	}

	s.sessions = sessions
	s.filtered = sessions
	s.active = true
	s.selectedIdx = 0
	s.searchQuery = ""
	s.scrollOffset = 0
	s.width = width
	s.height = height

	return nil
}

// IsActive returns whether the selector is active
func (s *SessionSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *SessionSelectorState) Cancel() {
	s.active = false
	s.sessions = nil
	s.filtered = nil
	s.selectedIdx = 0
	s.searchQuery = ""
	s.scrollOffset = 0
}

// MoveUp moves the selection up
func (s *SessionSelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *SessionSelectorState) MoveDown() {
	if s.selectedIdx < len(s.filtered)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *SessionSelectorState) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters sessions based on search query
func (s *SessionSelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filtered = s.sessions
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filtered = make([]*session.SessionMetadata, 0)
		for _, sess := range s.sessions {
			if fuzzyMatch(strings.ToLower(sess.Title), query) ||
				fuzzyMatch(strings.ToLower(sess.Model), query) ||
				fuzzyMatch(strings.ToLower(sess.Cwd), query) {
				s.filtered = append(s.filtered, sess)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// Select returns a command when a session is selected
func (s *SessionSelectorState) Select() tea.Cmd {
	if len(s.filtered) == 0 || s.selectedIdx >= len(s.filtered) {
		return nil
	}

	selected := s.filtered[s.selectedIdx]
	s.active = false

	return func() tea.Msg {
		return SessionSelectedMsg{SessionID: selected.ID}
	}
}

// HandleKeypress handles a keypress and returns a command if selection is made
func (s *SessionSelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter:
		return s.Select()
	case tea.KeyEsc:
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg {
			return SessionSelectorCancelledMsg{}
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

	// Vim-style navigation (only when not searching)
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

// Render renders the session selector
func (s *SessionSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with count
	title := fmt.Sprintf("Resume Session (%d/%d)", len(s.filtered), len(s.sessions))
	sb.WriteString(selectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	if s.searchQuery == "" {
		sb.WriteString(selectorHintStyle.Render("Type to filter..."))
	} else {
		sb.WriteString(selectorBreadcrumbStyle.Render("> " + s.searchQuery + "_"))
	}
	sb.WriteString("\n\n")

	// Handle empty results
	if len(s.filtered) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No sessions match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := s.scrollOffset + s.maxVisible
		if endIdx > len(s.filtered) {
			endIdx = len(s.filtered)
		}

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(selectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		// Render visible sessions
		dateStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
		countStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)

		for i := s.scrollOffset; i < endIdx; i++ {
			sess := s.filtered[i]

			// Format: Title (date) [messages]
			dateStr := formatRelativeTime(sess.UpdatedAt)
			msgCount := fmt.Sprintf("[%d msgs]", sess.MessageCount)

			// Truncate title if needed (larger than standard selectors)
			maxTitleLen := 60
			title := sess.Title
			if len(title) > maxTitleLen {
				title = title[:maxTitleLen-3] + "..."
			}

			line := fmt.Sprintf("%s %s %s",
				title,
				dateStyle.Render(dateStr),
				countStyle.Render(msgCount),
			)

			if i == s.selectedIdx {
				sb.WriteString(selectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(selectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filtered) {
			sb.WriteString(selectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(selectorHintStyle.Render("↑/↓ navigate · Enter select · Esc clear/cancel"))

	// Wrap in border (use larger width for session selector)
	content := sb.String()
	box := selectorBorderStyle.Width(calculateSessionBoxWidth(s.width)).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// formatRelativeTime formats a time as a relative string (e.g., "2h ago", "yesterday")
func formatRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2")
	}
}

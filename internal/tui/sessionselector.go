package tui

import (
	"fmt"
	"path/filepath"
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
	store        *session.Store
	cwd          string
	messageCache map[string]string // Cache for last user messages
}

// NewSessionSelectorState creates a new SessionSelectorState
func NewSessionSelectorState() SessionSelectorState {
	return SessionSelectorState{
		active:       false,
		maxVisible:   6, // Default, will be calculated based on terminal height
		messageCache: make(map[string]string),
	}
}

// clamp constrains a value between min and max bounds
func clamp(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

// calculateMaxVisible calculates how many sessions can fit on screen.
// Each session takes 3 lines (title + preview + blank separator).
func calculateMaxVisible(height int) int {
	const (
		fixedLines      = 7 // title(1) + search(1) + blank(1) + hint(2) + scroll indicators(2)
		linesPerSession = 3
	)
	maxVisible := (height - fixedLines) / linesPerSession
	return clamp(maxVisible, 3, 20)
}

// calculateMessagePreviewLength calculates message preview length based on terminal width.
// Accounts for indentation (4 chars) + quotes (2 chars) + margins.
func calculateMessagePreviewLength(width int) int {
	return clamp(width-10, 30, 120)
}

// EnterSessionSelect enters session selection mode
func (s *SessionSelectorState) EnterSessionSelect(width, height int, store *session.Store, cwd string) error {
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

	*s = SessionSelectorState{
		active:       true,
		sessions:     sessions,
		width:        width,
		height:       height,
		maxVisible:   calculateMaxVisible(height),
		store:        store,
		cwd:          cwd,
		messageCache: make(map[string]string),
	}
	s.updateFilter()

	if len(s.filtered) == 0 {
		return fmt.Errorf("no sessions found for current directory")
	}
	return nil
}

// IsActive returns whether the selector is active
func (s *SessionSelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *SessionSelectorState) Cancel() {
	*s = NewSessionSelectorState()
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

// updateFilter filters sessions by CWD and search query
func (s *SessionSelectorState) updateFilter() {
	query := strings.ToLower(s.searchQuery)
	s.filtered = make([]*session.SessionMetadata, 0, len(s.sessions))

	for _, sess := range s.sessions {
		if sess.Cwd != s.cwd {
			continue
		}
		if query != "" && !fuzzyMatch(strings.ToLower(sess.Title), query) &&
			!fuzzyMatch(strings.ToLower(sess.Model), query) {
			continue
		}
		s.filtered = append(s.filtered, sess)
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
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
	case tea.KeyEnter:
		return s.Select()
	case tea.KeyEsc:
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return SessionSelectorCancelledMsg{} }
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
	case tea.KeyRunes:
		if s.searchQuery == "" && (key.String() == "j" || key.String() == "k") {
			if key.String() == "j" {
				s.MoveDown()
			} else {
				s.MoveUp()
			}
			return nil
		}
		s.searchQuery += string(key.Runes)
		s.updateFilter()
	}
	return nil
}

// formatCompactMetadata formats message count and time inline
func formatCompactMetadata(sess *session.SessionMetadata) string {
	return fmt.Sprintf("%d msgs · %s", sess.MessageCount, formatRelativeTime(sess.UpdatedAt))
}

// truncateToFirstLine extracts the first line and truncates to maxLen
func truncateToFirstLine(content string, maxLen int) string {
	content = strings.TrimSpace(content)
	if first, _, found := strings.Cut(content, "\n"); found {
		content = first
	}
	if len(content) > maxLen {
		return content[:maxLen-3] + "..."
	}
	return content
}

// getLastUserMessage retrieves the last user message from a session for preview
func (s *SessionSelectorState) getLastUserMessage(sess *session.SessionMetadata) string {
	if cached, ok := s.messageCache[sess.ID]; ok {
		return cached
	}

	if s.store == nil {
		return ""
	}

	fullSession, err := s.store.Load(sess.ID)
	if err != nil {
		return ""
	}

	for i := len(fullSession.Messages) - 1; i >= 0; i-- {
		msg := fullSession.Messages[i]
		if msg.Role == "user" && msg.Content != "" {
			maxLen := calculateMessagePreviewLength(s.width)
			content := truncateToFirstLine(msg.Content, maxLen)
			s.messageCache[sess.ID] = content
			return content
		}
	}

	return ""
}

// renderSession renders a single session in compact 2-line format
func (s *SessionSelectorState) renderSession(sess *session.SessionMetadata, isSelected bool, sb *strings.Builder, boxWidth int) {
	titleStyle, indent := selectorItemStyle, "  "
	if isSelected {
		titleStyle, indent = selectorSelectedStyle, "> "
	}

	metadata := formatCompactMetadata(sess)
	title := truncateWithEllipsis(sess.Title, boxWidth-len(indent)-len(" · ")-len(metadata)-2)
	sb.WriteString(titleStyle.Render(fmt.Sprintf("%s%s · %s", indent, title, metadata)) + "\n")

	if lastMsg := s.getLastUserMessage(sess); lastMsg != "" {
		previewStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
		sb.WriteString(previewStyle.Render(fmt.Sprintf("    \"%s\"", lastMsg)))
	}
	sb.WriteString("\n\n")
}

// Render renders the session selector
func (s *SessionSelectorState) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with project name and count
	title := fmt.Sprintf("Resume Session - %s (%d/%d)", filepath.Base(s.cwd), len(s.filtered), len(s.sessions))
	sb.WriteString(selectorTitleStyle.Render(title) + "\n")

	// Search input
	searchLine := "🔍 Type to filter..."
	searchStyle := selectorHintStyle
	if s.searchQuery != "" {
		searchLine = "> " + s.searchQuery + "_"
		searchStyle = selectorBreadcrumbStyle
	}
	sb.WriteString(searchStyle.Render(searchLine) + "\n\n")

	if len(s.filtered) == 0 {
		sb.WriteString(selectorHintStyle.Render("  No sessions match the filter") + "\n")
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filtered))
		s.renderScrollIndicator(&sb, s.scrollOffset > 0, "↑ more above")

		for i := s.scrollOffset; i < endIdx; i++ {
			s.renderSession(s.filtered[i], i == s.selectedIdx, &sb, s.width)
		}

		s.renderScrollIndicator(&sb, endIdx < len(s.filtered), "↓ more below")
	}

	sb.WriteString("\n" + selectorHintStyle.Render("↑/↓ navigate · Enter select · Esc clear/cancel"))
	return sb.String()
}

// renderScrollIndicator writes a scroll indicator if the condition is true
func (s *SessionSelectorState) renderScrollIndicator(sb *strings.Builder, show bool, text string) {
	if show {
		sb.WriteString(selectorHintStyle.Render("  "+text) + "\n")
	}
}

// formatRelativeTime formats a time as a relative string (e.g., "2h ago", "yesterday")
func formatRelativeTime(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return pluralize(int(diff.Minutes()), "min") + " ago"
	case diff < 24*time.Hour:
		return pluralize(int(diff.Hours()), "hour") + " ago"
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(diff.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// pluralize returns "1 unit" or "n units" based on count
func pluralize(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

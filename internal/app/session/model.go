// Package session provides the session selector feature.
package session

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// SelectedMsg is sent when a session is selected
type SelectedMsg struct {
	SessionID string
}

// Model holds the state for the session selector
type Model struct {
	active       bool
	sessions     []*SessionMetadata
	filtered     []*SessionMetadata
	selectedIdx  int
	searchQuery  string
	scrollOffset int
	maxVisible   int
	width        int
	height       int
	store        *Store
	cwd          string
	messageCache map[string]string // Cache for last user messages
}

// New creates a new Model
func New() Model {
	return Model{
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

// EnterSelect enters session selection mode
func (s *Model) EnterSelect(width, height int, store *Store, cwd string) error {
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

	*s = Model{
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
		return fmt.Errorf("no sessions match the filter")
	}
	return nil
}

// IsActive returns whether the selector is active
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *Model) Cancel() {
	*s = New()
}

// MoveUp moves the selection up
func (s *Model) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *Model) MoveDown() {
	if s.selectedIdx < len(s.filtered)-1 {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *Model) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters sessions by search query.
// The store is already project-scoped, so no CWD filtering is needed.
func (s *Model) updateFilter() {
	query := strings.ToLower(s.searchQuery)
	s.filtered = make([]*SessionMetadata, 0, len(s.sessions))

	for _, sess := range s.sessions {
		if query != "" && !shared.FuzzyMatch(strings.ToLower(sess.Title), query) &&
			!shared.FuzzyMatch(strings.ToLower(sess.Model), query) {
			continue
		}
		s.filtered = append(s.filtered, sess)
	}

	s.selectedIdx = 0
	s.scrollOffset = 0
}

// Select returns a command when a session is selected
func (s *Model) Select() tea.Cmd {
	if len(s.filtered) == 0 || s.selectedIdx >= len(s.filtered) {
		return nil
	}

	selected := s.filtered[s.selectedIdx]
	s.active = false

	return func() tea.Msg {
		return SelectedMsg{SessionID: selected.ID}
	}
}

// HandleKeypress handles a keypress and returns a command if selection is made
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
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
		return func() tea.Msg { return shared.DismissedMsg{} }
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
func formatCompactMetadata(sess *SessionMetadata) string {
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

// getLastMessage retrieves the last message (user or assistant) from a session
// for preview. It skips tool_result and tool_use entries.
func (s *Model) getLastMessage(sess *SessionMetadata) string {
	cacheKey := sess.ID + ":last"
	if cached, ok := s.messageCache[cacheKey]; ok {
		return cached
	}

	if sess.LastPrompt != "" {
		maxLen := calculateMessagePreviewLength(s.width)
		content := truncateToFirstLine(sess.LastPrompt, maxLen)
		s.messageCache[cacheKey] = content
		return content
	}

	if s.store == nil {
		return ""
	}

	fullSession, err := s.store.Load(sess.ID)
	if err != nil {
		return ""
	}

	for i := len(fullSession.Entries) - 1; i >= 0; i-- {
		entry := fullSession.Entries[i]
		if entry.Message == nil {
			continue
		}
		if entry.Type != EntryUser && entry.Type != EntryAssistant {
			continue
		}
		// Skip tool_result and tool_use entries.
		skip := false
		for _, block := range entry.Message.Content {
			if block.Type == "tool_result" || block.Type == "tool_use" {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		for _, block := range entry.Message.Content {
			if block.Type == "text" && block.Text != "" {
				maxLen := calculateMessagePreviewLength(s.width)
				content := truncateToFirstLine(block.Text, maxLen)
				s.messageCache[cacheKey] = content
				return content
			}
		}
	}

	return ""
}

// getFirstSubstantiveMessage finds the first user message with >5 characters
// from a session. Used as a display title when the stored title is too short.
func (s *Model) getFirstSubstantiveMessage(sess *SessionMetadata) string {
	cacheKey := sess.ID + ":subst"
	if cached, ok := s.messageCache[cacheKey]; ok {
		return cached
	}

	if s.store == nil {
		return ""
	}

	fullSession, err := s.store.Load(sess.ID)
	if err != nil {
		return ""
	}

	for _, entry := range fullSession.Entries {
		if entry.Type != EntryUser || entry.Message == nil {
			continue
		}
		// Skip tool_result entries.
		isToolResult := false
		for _, block := range entry.Message.Content {
			if block.Type == "tool_result" {
				isToolResult = true
				break
			}
		}
		if isToolResult {
			continue
		}
		for _, block := range entry.Message.Content {
			if block.Type == "text" && len([]rune(block.Text)) >= MinSubstantiveLength {
				maxLen := calculateMessagePreviewLength(s.width)
				content := truncateToFirstLine(block.Text, maxLen)
				s.messageCache[cacheKey] = content
				return content
			}
		}
	}

	return ""
}

// renderSession renders a single session in compact 2-line format.
//
// Title line: if the stored title is too short (≤5 chars, e.g. "hi") and a
// substantive message exists, the substantive message is used as the display
// title. Metadata (message count + relative time) is right-aligned.
//
// Subtitle line: the last message in the conversation (any role) is shown as
// a muted preview.
func (s *Model) renderSession(sess *SessionMetadata, isSelected bool, sb *strings.Builder, boxWidth int) {
	titleStyle, indent := shared.SelectorItemStyle, "  "
	if isSelected {
		titleStyle, indent = shared.SelectorSelectedStyle, "> "
	}

	// Determine display title — prefer substantive message over short titles.
	displayTitle := sess.Title
	if len([]rune(displayTitle)) < MinSubstantiveLength {
		if subst := s.getFirstSubstantiveMessage(sess); subst != "" {
			displayTitle = subst
		}
	}

	metadata := formatCompactMetadata(sess)
	// Reserve space for indent + gap + metadata.
	maxTitleWidth := boxWidth - len(indent) - len(metadata) - 4
	if maxTitleWidth < 10 {
		maxTitleWidth = 10
	}
	title := shared.TruncateWithEllipsis(displayTitle, maxTitleWidth)

	// Right-align metadata by padding between title and metadata.
	titleLen := len(indent) + len(title)
	gap := boxWidth - titleLen - len(metadata) - 2
	if gap < 2 {
		gap = 2
	}
	padding := strings.Repeat(" ", gap)
	sb.WriteString(titleStyle.Render(fmt.Sprintf("%s%s%s%s", indent, title, padding, metadata)) + "\n")

	if lastMsg := s.getLastMessage(sess); lastMsg != "" {
		previewStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
		sb.WriteString(previewStyle.Render(fmt.Sprintf("    %s", lastMsg)))
	}
	sb.WriteString("\n\n")
}

// Render renders the session selector
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	// Title with project name and count
	title := fmt.Sprintf("Resume Session - %s (%d/%d)", filepath.Base(s.cwd), len(s.filtered), len(s.sessions))
	sb.WriteString(shared.SelectorTitleStyle.Render(title) + "\n")

	// Search input
	searchLine := "🔍 Type to filter..."
	searchStyle := shared.SelectorHintStyle
	if s.searchQuery != "" {
		searchLine = "> " + s.searchQuery + "_"
		searchStyle = shared.SelectorBreadcrumbStyle
	}
	sb.WriteString(searchStyle.Render(searchLine) + "\n\n")

	if len(s.filtered) == 0 {
		sb.WriteString(shared.SelectorHintStyle.Render("  No sessions match the filter") + "\n")
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.filtered))
		s.renderScrollIndicator(&sb, s.scrollOffset > 0, "↑ more above")

		for i := s.scrollOffset; i < endIdx; i++ {
			s.renderSession(s.filtered[i], i == s.selectedIdx, &sb, s.width)
		}

		s.renderScrollIndicator(&sb, endIdx < len(s.filtered), "↓ more below")
	}

	sb.WriteString("\n" + shared.SelectorHintStyle.Render("↑/↓ navigate · Enter select · Esc clear/cancel"))
	return sb.String()
}

// renderScrollIndicator writes a scroll indicator if the condition is true
func (s *Model) renderScrollIndicator(sb *strings.Builder, show bool, text string) {
	if show {
		sb.WriteString(shared.SelectorHintStyle.Render("  "+text) + "\n")
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

// Keyboard handling for the MCP server selector UI.
package mcpui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
)

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Only allow escape while connecting
	if s.connecting {
		if key.Type == tea.KeyEsc {
			s.Cancel()
			return func() tea.Msg { return kit.DismissedMsg{} }
		}
		return nil
	}

	// Detail view keypress handling
	if s.level == levelDetail {
		return s.handleDetailKeypress(key)
	}

	// List view keypress handling
	return s.handleListKeypress(key)
}

// handleDetailKeypress handles keypresses in the detail view
func (s *Model) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter:
		return s.executeAction()
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
		return nil
	case tea.KeyRunes:
		switch key.String() {
		case "k":
			s.MoveUp()
		case "j":
			s.MoveDown()
		case "h":
			s.goBack()
		}
		return nil
	}
	return nil
}

// handleListKeypress handles keypresses in the list view
func (s *Model) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlJ:
		s.MoveDown()
		return nil
	case tea.KeyCtrlN:
		s.Cancel()
		return func() tea.Msg { return AddServerMsg{} }
	case tea.KeyCtrlD:
		if len(s.filteredServers) > 0 && s.nav.Selected < len(s.filteredServers) {
			name := s.filteredServers[s.nav.Selected].Name
			return func() tea.Msg { return RemoveMsg{ServerName: name} }
		}
		return nil
	case tea.KeyEnter, tea.KeyRight:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		if s.nav.Search != "" {
			s.nav.Search = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.nav.Search) > 0 {
			s.nav.Search = s.nav.Search[:len(s.nav.Search)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		r := key.String()
		// vim navigation when not searching
		if s.nav.Search == "" {
			switch r {
			case "j":
				s.MoveDown()
				return nil
			case "k":
				s.MoveUp()
				return nil
			case "l":
				s.enterDetail()
				return nil
			}
		}
		s.nav.Search += r
		s.updateFilter()
		return nil
	}
	return nil
}

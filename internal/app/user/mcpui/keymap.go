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
		if len(s.filteredServers) > 0 && s.selectedIdx < len(s.filteredServers) {
			name := s.filteredServers[s.selectedIdx].Name
			return func() tea.Msg { return RemoveMsg{ServerName: name} }
		}
		return nil
	case tea.KeyEnter, tea.KeyRight:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		// First clear search if active
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		// Then close the selector
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		r := key.String()
		// vim navigation when not searching
		if s.searchQuery == "" {
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
		// Append to search query
		s.searchQuery += r
		s.updateFilter()
		return nil
	}
	return nil
}

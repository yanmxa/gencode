package input

import (
	tea "github.com/charmbracelet/bubbletea"
)

// HandleImageSelectKey handles inline image token selection and deletion.
// Returns (cmd, true) if the key was consumed, (nil, false) otherwise.
func (m *Model) HandleImageSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if len(m.Images.Pending) == 0 {
		return nil, false
	}

	if m.Images.Selection.Active {
		match, ok := m.SelectedImageMatch()
		if !ok {
			m.Images.Selection = ImageSelection{}
			return nil, false
		}
		switch msg.Type {
		case tea.KeyLeft:
			if m.Images.Selection.CursorAbsPos == match.End {
				m.SetCursorIndex(match.Start)
			}
			m.Images.Selection = ImageSelection{}
			return nil, true
		case tea.KeyRight:
			if m.Images.Selection.CursorAbsPos == match.Start {
				m.SetCursorIndex(match.End)
			}
			m.Images.Selection = ImageSelection{}
			return nil, true
		case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlX:
			m.RemoveImageToken(match, match.Start)
			return nil, true
		case tea.KeyEsc:
			m.Images.Selection = ImageSelection{}
			return nil, true
		}

		m.Images.Selection = ImageSelection{}
		return nil, false
	}

	cursor := m.CursorIndex()
	switch msg.Type {
	case tea.KeyLeft:
		if match, ok := m.MatchAdjacentToCursor(cursor, false); ok {
			m.Images.Selection = ImageSelection{
				Active:       true,
				PendingIdx:   match.PendingIdx,
				CursorAbsPos: cursor,
			}
			return nil, true
		}
	case tea.KeyRight:
		if match, ok := m.MatchAdjacentToCursor(cursor, true); ok {
			m.Images.Selection = ImageSelection{
				Active:       true,
				PendingIdx:   match.PendingIdx,
				CursorAbsPos: cursor,
			}
			return nil, true
		}
	case tea.KeyBackspace, tea.KeyCtrlX:
		if match, ok := m.MatchAdjacentToCursor(cursor, false); ok {
			m.RemoveImageToken(match, match.Start)
			return nil, true
		}
	case tea.KeyDelete:
		if match, ok := m.MatchAdjacentToCursor(cursor, true); ok {
			m.RemoveImageToken(match, match.Start)
			return nil, true
		}
	}

	return nil, false
}

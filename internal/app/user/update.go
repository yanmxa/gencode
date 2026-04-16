package user

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/ui/suggest"
)

// ExtractPastedText derives the pasted content by comparing the textarea
// value before and after the paste event.
func ExtractPastedText(prevValue, newValue string) string {
	if strings.HasPrefix(newValue, prevValue) {
		return strings.TrimSpace(newValue[len(prevValue):])
	}
	return strings.TrimSpace(newValue)
}

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

// HandleSuggestionKey handles keys while the autocomplete suggestion list is visible.
// Returns (cmd, true) if the key was consumed, (nil, false) otherwise.
func (m *Model) HandleSuggestionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.Suggestions.IsVisible() {
		return nil, false
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		m.Suggestions.MoveUp()
		return nil, true
	case tea.KeyDown, tea.KeyCtrlN:
		m.Suggestions.MoveDown()
		return nil, true
	case tea.KeyTab, tea.KeyEnter:
		if selected := m.Suggestions.GetSelected(); selected != "" {
			if m.Suggestions.GetSuggestionType() == suggest.TypeFile {
				currentValue := m.Textarea.Value()
				if atIdx := strings.LastIndex(currentValue, "@"); atIdx >= 0 {
					newValue := currentValue[:atIdx] + "@" + selected
					m.Textarea.SetValue(newValue)
					m.Textarea.CursorEnd()
				}
			} else {
				m.Textarea.SetValue(selected + " ")
				m.Textarea.CursorEnd()
			}
			m.Suggestions.Hide()
		}
		return nil, true
	case tea.KeyEsc:
		m.Suggestions.Hide()
		return nil, true
	}
	return nil, false
}

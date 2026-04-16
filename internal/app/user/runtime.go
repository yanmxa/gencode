package user

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// HandleTextareaUpdate forwards a message to the textarea and applies user-input
// side effects such as paste placeholder expansion, height updates, and suggestions.
// It returns the resulting tea.Cmd and whether the textarea value changed.
func HandleTextareaUpdate(m *Model, msg tea.Msg) (tea.Cmd, bool) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	isPaste := false
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		isPaste = keyMsg.Paste
	}

	prevValue := m.Textarea.Value()
	m.Textarea, cmd = m.Textarea.Update(msg)
	cmds = append(cmds, cmd)

	if isPaste {
		newValue := m.Textarea.Value()
		pastedText := ExtractPastedText(prevValue, newValue)
		lines := strings.Split(pastedText, "\n")
		if len(lines) > 1 {
			chunk := PastedChunk{
				Text:      pastedText,
				LineCount: len(lines),
			}
			m.PastedChunks = append(m.PastedChunks, chunk)
			placeholder := PastePlaceholder(len(m.PastedChunks), chunk.LineCount)
			m.Textarea.SetValue(prevValue)
			m.Textarea.CursorEnd()
			m.Textarea.InsertString(placeholder)
		} else {
			trimmed := strings.TrimSpace(newValue)
			if trimmed != newValue {
				m.Textarea.SetValue(trimmed)
				m.Textarea.CursorEnd()
			}
		}
	}

	changed := m.Textarea.Value() != prevValue
	if changed {
		m.UpdateHeight()
		m.Suggestions.UpdateSuggestions(m.Textarea.Value())
	}

	return tea.Batch(cmds...), changed
}

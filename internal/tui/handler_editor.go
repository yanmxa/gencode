package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleEditorFinished(msg EditorFinishedMsg) (tea.Model, tea.Cmd) {
	filePath := m.editingMemoryFile
	m.editingMemoryFile = ""

	m.cachedMemory = ""

	content := fmt.Sprintf("Saved: %s", filePath)
	if msg.Err != nil {
		content = fmt.Sprintf("Editor error: %v", msg.Err)
	}

	m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})
	return m, tea.Batch(m.commitMessages()...)
}

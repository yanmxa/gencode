package app

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	"github.com/yanmxa/gencode/internal/message"
)

// startExternalEditor is a thin wrapper that delegates to memory.StartExternalEditor
// and converts the result into an appmemory.EditorFinishedMsg for the app message loop.
func startExternalEditor(filePath string) tea.Cmd {
	return appmemory.StartExternalEditor(filePath, func(err error) tea.Msg {
		return appmemory.EditorFinishedMsg{Err: err}
	})
}

// updateMemory routes memory selection and editor messages.
func (m *model) updateMemory(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appmemory.SelectedMsg:
		c := m.handleMemorySelected(msg)
		return c, true
	case appmemory.EditorFinishedMsg:
		c := m.handleEditorFinished(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handleMemorySelected(msg appmemory.SelectedMsg) tea.Cmd {
	filePath := msg.Path

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := appmemory.CreateMemoryFile(filePath, msg.Level, m.cwd); err != nil {
			m.conv.Append(message.ChatMessage{
				Role:    message.RoleNotice,
				Content: fmt.Sprintf("Error: %v", err),
			})
			return tea.Batch(m.commitMessages()...)
		}
	}

	m.memory.EditingFile = filePath

	displayPath := appmemory.FormatMemoryDisplayPath(filePath, msg.Level, m.cwd)

	m.conv.Append(message.ChatMessage{
		Role:    message.RoleNotice,
		Content: fmt.Sprintf("Opening %s memory: %s", msg.Level, displayPath),
	})

	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, startExternalEditor(filePath))
	return tea.Batch(commitCmds...)
}

func (m *model) handleEditorFinished(msg appmemory.EditorFinishedMsg) tea.Cmd {
	filePath := m.memory.EditingFile
	m.memory.EditingFile = ""

	m.memory.CachedUser = ""
	m.memory.CachedProject = ""

	content := fmt.Sprintf("Saved: %s", filePath)
	if msg.Err != nil {
		content = fmt.Sprintf("Editor error: %v", msg.Err)
	}

	m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: content})
	return tea.Batch(m.commitMessages()...)
}

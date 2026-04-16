package memory

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
)

// Runtime defines the callbacks the memory package needs from the parent app model.
type Runtime interface {
	AppendMessage(msg core.ChatMessage)
	CommitMessages() []tea.Cmd
	GetCwd() string
	ClearCachedInstructions()
	RefreshMemoryContext(trigger string)
	FireFileChanged(path, tool string)
}

// Update routes memory selection and editor messages.
func Update(rt Runtime, state *State, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case SelectedMsg:
		return handleMemorySelected(rt, state, msg), true
	case EditorFinishedMsg:
		return handleEditorFinished(rt, state, msg), true
	}
	return nil, false
}

func handleMemorySelected(rt Runtime, state *State, msg SelectedMsg) tea.Cmd {
	filePath := msg.Path

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := CreateMemoryFile(filePath, msg.Level, rt.GetCwd()); err != nil {
			rt.AppendMessage(core.ChatMessage{
				Role:    core.RoleNotice,
				Content: fmt.Sprintf("Error: %v", err),
			})
			return tea.Batch(rt.CommitMessages()...)
		}
	}

	state.EditingFile = filePath

	displayPath := FormatMemoryDisplayPath(filePath, msg.Level, rt.GetCwd())

	rt.AppendMessage(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: fmt.Sprintf("Opening %s memory: %s", msg.Level, displayPath),
	})

	commitCmds := rt.CommitMessages()
	commitCmds = append(commitCmds, startExternalEditorForMemory(filePath))
	return tea.Batch(commitCmds...)
}

func handleEditorFinished(rt Runtime, state *State, msg EditorFinishedMsg) tea.Cmd {
	filePath := state.EditingFile
	state.EditingFile = ""

	rt.ClearCachedInstructions()

	content := fmt.Sprintf("Saved: %s", filePath)
	if msg.Err != nil {
		content = fmt.Sprintf("Editor error: %v", msg.Err)
	} else {
		rt.RefreshMemoryContext("memory_edit")
		rt.FireFileChanged(filePath, "memory_editor")
	}

	rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: content})
	return tea.Batch(rt.CommitMessages()...)
}

// startExternalEditorForMemory launches the external editor for a memory file.
func startExternalEditorForMemory(filePath string) tea.Cmd {
	return kit.StartExternalEditor(filePath, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

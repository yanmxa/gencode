package mcpui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/user/memory"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/mcp"
)

// Runtime defines the callbacks the mcpui package needs from the parent app model.
type Runtime interface {
	AppendMessage(msg core.ChatMessage)
	CommitMessages() []tea.Cmd
	SetInputText(text string)
}

// Update routes MCP server management messages.
func Update(rt Runtime, state *State, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ConnectMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetDisabled(msg.ServerName, false)
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return StartConnect(msg.ServerName), true

	case ConnectResultMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, false)
			if !msg.Success && msg.Error != nil {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, msg.Error.Error())
			} else {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, "")
			}
		}
		state.Selector.HandleConnectResult(msg)
		if !state.Selector.IsActive() && !msg.Success {
			content := fmt.Sprintf("Failed to connect to '%s': %v", msg.ServerName, msg.Error)
			rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: content})
			return tea.Batch(rt.CommitMessages()...), true
		}
		return nil, true

	case DisconnectMsg:
		state.Selector.HandleDisconnect(msg.ServerName)
		return nil, true

	case ReconnectMsg:
		state.Selector.HandleReconnect(msg.ServerName)
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return StartConnect(msg.ServerName), true

	case RemoveMsg:
		state.Selector.HandleRemove(msg.ServerName)
		return nil, true

	case AddServerMsg:
		rt.SetInputText("/mcp add ")
		return nil, true

	case EditServerMsg:
		info, err := PrepareServerEdit(msg.ServerName)
		if err != nil {
			rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Error: %v", err)})
			return tea.Batch(rt.CommitMessages()...), true
		}
		state.EditingFile = info.TempFile
		state.EditingServer = info.ServerName
		state.EditingScope = info.Scope
		return StartMCPEditor(info.TempFile), true

	case EditorFinishedMsg:
		info := &EditInfo{
			TempFile:   state.EditingFile,
			ServerName: state.EditingServer,
			Scope:      state.EditingScope,
		}
		state.EditingFile, state.EditingServer, state.EditingScope = "", "", ""

		if msg.Err != nil {
			os.Remove(info.TempFile)
			rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Editor error: %v", msg.Err)})
			return tea.Batch(rt.CommitMessages()...), true
		}

		if err := ApplyServerEdit(info); err != nil {
			rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Failed to apply edit: %v", err)})
			return tea.Batch(rt.CommitMessages()...), true
		}

		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Updated MCP server '%s'", info.ServerName)})
		return tea.Batch(rt.CommitMessages()...), true
	}
	return nil, false
}

// StartMCPEditor launches the external editor for an MCP config file.
// Exported for use by command handlers in the parent app package.
func StartMCPEditor(filePath string) tea.Cmd {
	return memory.StartExternalEditor(filePath, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

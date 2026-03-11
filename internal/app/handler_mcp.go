package app

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
)

// updateMCP routes MCP server management messages.
func (m *model) updateMCP(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appmcp.ConnectMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetDisabled(msg.ServerName, false)
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return appmcp.StartConnect(msg.ServerName), true

	case appmcp.ConnectResultMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, false)
			if !msg.Success && msg.Error != nil {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, msg.Error.Error())
			} else {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, "")
			}
		}
		m.mcp.Selector.HandleConnectResult(msg)
		if !m.mcp.Selector.IsActive() && !msg.Success {
			content := fmt.Sprintf("Failed to connect to '%s': %v", msg.ServerName, msg.Error)
			m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: content})
			return tea.Batch(m.commitMessages()...), true
		}
		return nil, true

	case appmcp.DisconnectMsg:
		m.mcp.Selector.HandleDisconnect(msg.ServerName)
		return nil, true

	case appmcp.ReconnectMsg:
		m.mcp.Selector.HandleReconnect(msg.ServerName)
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return appmcp.StartConnect(msg.ServerName), true

	case appmcp.RemoveMsg:
		m.mcp.Selector.HandleRemove(msg.ServerName)
		return nil, true

	case appmcp.AddServerMsg:
		m.input.Textarea.SetValue("/mcp add ")
		return nil, true

	case appmcp.EditServerMsg:
		info, err := appmcp.PrepareServerEdit(msg.ServerName)
		if err != nil {
			m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: fmt.Sprintf("Error: %v", err)})
			return tea.Batch(m.commitMessages()...), true
		}
		m.mcp.EditingFile = info.TempFile
		m.mcp.EditingServer = info.ServerName
		m.mcp.EditingScope = info.Scope
		return startMCPEditor(info.TempFile), true

	case appmcp.EditorFinishedMsg:
		info := &appmcp.EditInfo{
			TempFile:   m.mcp.EditingFile,
			ServerName: m.mcp.EditingServer,
			Scope:      m.mcp.EditingScope,
		}
		m.mcp.EditingFile, m.mcp.EditingServer, m.mcp.EditingScope = "", "", ""

		if msg.Err != nil {
			os.Remove(info.TempFile)
			m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: fmt.Sprintf("Editor error: %v", msg.Err)})
			return tea.Batch(m.commitMessages()...), true
		}

		if err := appmcp.ApplyServerEdit(info); err != nil {
			m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: fmt.Sprintf("Failed to apply edit: %v", err)})
			return tea.Batch(m.commitMessages()...), true
		}

		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: fmt.Sprintf("Updated MCP server '%s'", info.ServerName)})
		return tea.Batch(m.commitMessages()...), true
	}
	return nil, false
}

// startMCPEditor launches the external editor for an MCP config file.
func startMCPEditor(filePath string) tea.Cmd {
	return appmemory.StartExternalEditor(filePath, func(err error) tea.Msg {
		return appmcp.EditorFinishedMsg{Err: err}
	})
}

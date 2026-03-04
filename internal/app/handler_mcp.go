package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
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
	}
	return nil, false
}

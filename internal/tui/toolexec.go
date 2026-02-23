package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
)

func (m model) executeTools(toolCalls []message.ToolCall) tea.Cmd {
	return func() tea.Msg {
		return StartMsg{ToolCalls: toolCalls}
	}
}

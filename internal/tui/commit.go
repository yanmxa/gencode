// Message commit pipeline: pushes completed messages to terminal scrollback via tea.Println.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) commitMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(true)
}

func (m *model) commitAllMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(false)
}

func (m *model) commitMessagesWithCheck(checkReady bool) []tea.Cmd {
	var cmds []tea.Cmd
	lastIdx := len(m.messages) - 1

	for i := m.committedCount; i < len(m.messages); i++ {
		msg := m.messages[i]

		if checkReady {
			if i == lastIdx && msg.role == roleAssistant && m.stream.active {
				break
			}
			if msg.role == roleAssistant && len(msg.toolCalls) > 0 && !m.hasAllToolResults(i) {
				break
			}
		}

		if rendered := m.renderSingleMessage(i); rendered != "" {
			cmds = append(cmds, tea.Println(rendered))
		}
		m.committedCount = i + 1
	}
	return cmds
}

func (m *model) hasAllToolResults(idx int) bool {
	toolCalls := m.messages[idx].toolCalls
	if len(toolCalls) == 0 {
		return true
	}
	endIdx := idx + 1 + len(toolCalls)
	if endIdx > len(m.messages) {
		return false
	}
	for j := idx + 1; j < endIdx; j++ {
		if m.messages[j].toolResult == nil {
			return false
		}
	}
	return true
}

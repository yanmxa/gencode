package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleCompactResult(msg CompactResultMsg) (tea.Model, tea.Cmd) {
	shouldContinue := m.compact.autoContinue
	m.compact.Reset()

	if msg.Error != nil {
		m.addNotice(fmt.Sprintf("Compact failed: %v", msg.Error))
		return m, tea.Batch(m.commitMessages()...)
	}

	m.replaceWithSummary(msg.Summary, msg.OriginalCount)

	cmds := []tea.Cmd{tea.ClearScreen}
	if shouldContinue {
		m.messages = append(m.messages, chatMessage{
			role:    roleUser,
			content: "Continue with the task. The conversation was auto-compacted to free up context.",
		})
		cmds = append(cmds, m.startLLMStream(m.buildExtraContext()))
	} else {
		cmds = append(cmds, m.commitMessages()...)
	}
	return m, tea.Batch(cmds...)
}


// replaceWithSummary replaces all messages with a single summary message.
func (m *model) replaceWithSummary(summary string, originalCount int) {
	m.messages = []chatMessage{{
		role:         roleUser,
		content:      fmt.Sprintf("Here is a summary of our previous conversation:\n\n%s", summary),
		isSummary:    true,
		summaryCount: originalCount,
		expanded:     false,
	}}
	m.committedCount = 0
	m.lastInputTokens = 0
	m.lastOutputTokens = 0
}

// addNotice appends a notice message to the conversation.
func (m *model) addNotice(content string) {
	m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})
}

func (m *model) handleTokenLimitResult(msg TokenLimitResultMsg) (tea.Model, tea.Cmd) {
	m.fetchingTokenLimits = false

	var content string
	if msg.Error != nil {
		content = "Error: " + msg.Error.Error()
	} else {
		content = msg.Result
	}
	m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})

	return m, tea.Batch(m.commitMessages()...)
}

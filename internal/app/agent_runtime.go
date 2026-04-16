package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) updateAgentInput(msg tea.Msg) (tea.Cmd, bool) {
	return appagent.Update(m, &m.agentInput, msg)
}

func (m *model) StreamActive() bool {
	return m.conv.Stream.Active
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmd, _ := appagent.Update(m, &m.agentInput, appagent.TickMsg{})
	return cmd
}

func (m *model) InjectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	return m.injectTaskNotificationContinuation(item)
}

func (m *model) injectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if m.llmProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleNotice,
				Content: "A background task completed, but no provider is connected.",
			})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "A background task completed, but no task notification payload was available.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	for _, ctx := range appagent.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(appagent.BuildContinuationPrompt(item), nil)
}

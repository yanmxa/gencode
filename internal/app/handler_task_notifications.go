package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) updateTaskNotifications(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(appagent.TickMsg); !ok {
		return nil, false
	}
	return m.handleTaskNotificationTick(), true
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmds := []tea.Cmd{appagent.StartTicker()}

	appagent.ResetTrackerIfIdle(m.conv.Stream.Active)

	items := appagent.PopReadyNotifications(
		m.agentInput.Notifications,
		!m.conv.Stream.Active && !m.isToolPhaseActive(),
	)
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, m.injectTaskNotificationContinuation(appagent.MergeNotifications(items)))
	return tea.Batch(cmds...)
}

func (m *model) injectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if m.provider.LLM == nil {
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

	// Inject context as messages, then send the continuation prompt to the agent
	for _, ctx := range appagent.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(appagent.BuildContinuationPrompt(item), nil)
}


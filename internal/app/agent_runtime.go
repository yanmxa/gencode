package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/core"
)

type agentRuntime struct {
	m *model
}

func (m *model) updateAgentInput(msg tea.Msg) (tea.Cmd, bool) {
	return appagent.Update(agentRuntime{m: m}, &m.agentInput, msg)
}

func (m *model) streamActive() bool {
	return m.conv.Stream.Active
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmd, _ := appagent.Update(agentRuntime{m: m}, &m.agentInput, appagent.TickMsg{})
	return cmd
}

func (rt agentRuntime) IsInputIdle() bool {
	return rt.m.isInputIdle()
}

func (rt agentRuntime) StreamActive() bool {
	return rt.m.streamActive()
}

func (rt agentRuntime) InjectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	return rt.m.injectTaskNotificationContinuation(item)
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

package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/core"
)

type agentRuntime struct{ m *model }

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmd, _ := notify.Update(agentRuntime{m: m}, &m.agentInput, notify.TickMsg{})
	return cmd
}

func (rt agentRuntime) IsInputIdle() bool  { return rt.m.isInputIdle() }
func (rt agentRuntime) StreamActive() bool { return rt.m.conv.Stream.Active }
func (rt agentRuntime) InjectTaskNotificationContinuation(item notify.Notification) tea.Cmd {
	return rt.m.injectTaskNotificationContinuation(item)
}

func (m *model) injectTaskNotificationContinuation(item notify.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if m.runtime.LLMProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no provider is connected."})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no task notification payload was available."})
		return tea.Batch(m.commitMessages()...)
	}
	for _, ctx := range notify.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(notify.BuildContinuationPrompt(item), nil)
}

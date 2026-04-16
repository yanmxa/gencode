package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) DrainTurnQueues() tea.Cmd {
	for _, drain := range []func() tea.Cmd{
		m.drainInputQueueToAgent,
		m.drainCronQueueToAgent,
		m.drainAsyncHookQueueToAgent,
		m.drainTaskNotificationsToAgent,
	} {
		if cmd := drain(); cmd != nil {
			return cmd
		}
	}
	return nil
}

func (m *model) drainInputQueueToAgent() tea.Cmd {
	if m.userInput.Queue.Len() == 0 {
		return nil
	}
	item, ok := m.userInput.Queue.Dequeue()
	if !ok {
		return nil
	}
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: item.Content,
		Images:  item.Images,
	})
	return m.sendToAgent(item.Content, item.Images)
}

func (m *model) drainCronQueueToAgent() tea.Cmd {
	if len(m.systemInput.CronQueue) == 0 {
		return nil
	}
	prompt := m.systemInput.CronQueue[0]
	m.systemInput.CronQueue = m.systemInput.CronQueue[1:]

	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Scheduled task fired"})
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: prompt})
	return m.sendToAgent(prompt, nil)
}

func (m *model) drainAsyncHookQueueToAgent() tea.Cmd {
	if m.systemInput.AsyncHookQueue == nil {
		return nil
	}
	item, ok := m.systemInput.AsyncHookQueue.Pop()
	if !ok {
		return nil
	}

	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if len(item.Context) == 0 && item.ContinuationPrompt == "" {
		return nil
	}

	var content string
	if item.ContinuationPrompt != "" {
		content = item.ContinuationPrompt
	}
	for _, ctx := range item.Context {
		if content != "" {
			content += "\n"
		}
		content += ctx
	}

	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: content})
	return m.sendToAgent(content, nil)
}

func (m *model) drainTaskNotificationsToAgent() tea.Cmd {
	if m.agentInput.Notifications == nil {
		return nil
	}
	items := appagent.PopReadyNotifications(m.agentInput.Notifications, true)
	if len(items) == 0 {
		return nil
	}
	return m.injectTaskNotificationContinuation(appagent.MergeNotifications(items))
}

func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	inbox := m.agentSess.agent.Inbox()
	msg := core.Message{
		Role:    core.RoleUser,
		Content: content,
		Images:  images,
	}
	return func() tea.Msg {
		inbox <- msg
		return nil
	}
}

func (m *model) continueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return appoutput.DrainAgentOutbox(m.agentSess.agent.Outbox())
}

func (m *model) outputContinueOutbox() tea.Cmd {
	return m.continueOutbox()
}

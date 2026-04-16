package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
)

const asyncHookTickInterval = 500 * time.Millisecond

type asyncHookTickMsg struct{}

func startAsyncHookTicker() tea.Cmd {
	return tea.Tick(asyncHookTickInterval, func(time.Time) tea.Msg {
		return asyncHookTickMsg{}
	})
}

func (m *model) updateAsyncHooks(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(asyncHookTickMsg); !ok {
		return nil, false
	}
	return m.handleAsyncHookTick(), true
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmds := []tea.Cmd{startAsyncHookTicker()}
	if m.hookEngine != nil {
		m.hookStatus = m.hookEngine.CurrentStatusMessage()
	} else {
		m.hookStatus = ""
	}
	if m.conv.Stream.Active || m.isToolPhaseActive() {
		return tea.Batch(cmds...)
	}

	item, ok := m.asyncHookQueue.Pop()
	if !ok {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, m.injectAsyncHookContinuation(item))
	return tea.Batch(cmds...)
}

func (m *model) injectAsyncHookContinuation(item asyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: item.Notice,
		})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.provider.LLM == nil {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: "Async hook requested a follow-up, but no provider is connected.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	return m.startConversationStream(m.buildInternalContinuationRequest(item.Context, item.ContinuationPrompt))
}

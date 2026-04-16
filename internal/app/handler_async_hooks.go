package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) updateAsyncHooks(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(appsystem.AsyncHookTickMsg); !ok {
		return nil, false
	}
	return m.handleAsyncHookTick(), true
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmds := []tea.Cmd{appsystem.StartAsyncHookTicker()}
	idle := !m.conv.Stream.Active && !m.isToolPhaseActive()

	item := m.systemInput.HandleAsyncHookTick(m.hookEngine, idle)
	if item == nil {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, m.injectAsyncHookContinuation(*item))
	return tea.Batch(cmds...)
}

func (m *model) injectAsyncHookContinuation(item appsystem.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.provider.LLM == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Async hook requested a follow-up, but no provider is connected.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	// Inject context as system-reminder messages, then send the continuation prompt to the agent
	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

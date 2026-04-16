package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) updateSystemInput(msg tea.Msg) (tea.Cmd, bool) {
	return appsystem.Update(m, &m.systemInput, m.hookEngine, msg)
}

func (m *model) IsInputIdle() bool {
	return !m.conv.Stream.Active && !m.isToolPhaseActive()
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmd, _ := appsystem.Update(m, &m.systemInput, m.hookEngine, appsystem.AsyncHookTickMsg{})
	return cmd
}

func (m *model) InjectAsyncHookContinuation(item appsystem.AsyncHookRewake) tea.Cmd {
	return m.injectAsyncHookContinuation(item)
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

	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

func (m *model) InjectCronPrompt(prompt string) tea.Cmd {
	return m.injectCronPrompt(prompt)
}

func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.provider.LLM == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: "Scheduled task fired",
	})
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: prompt,
	})

	return m.sendToAgent(prompt, nil)
}

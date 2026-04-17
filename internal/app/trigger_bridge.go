package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/core"
)

type systemRuntime struct{ m *model }

func (m *model) isInputIdle() bool {
	return !m.conv.Stream.Active && !m.isToolPhaseActive()
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmd, _ := trigger.Update(systemRuntime{m: m}, &m.systemInput, trigger.AsyncHookTickMsg{})
	return cmd
}

func (rt systemRuntime) IsInputIdle() bool { return rt.m.isInputIdle() }
func (rt systemRuntime) InjectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	return rt.m.injectAsyncHookContinuation(item)
}
func (rt systemRuntime) InjectCronPrompt(prompt string) tea.Cmd { return rt.m.injectCronPrompt(prompt) }
func (rt systemRuntime) AppendNotice(text string) {
	if text == "" {
		return
	}
	rt.m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: text})
}

func (m *model) injectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Async hook requested a follow-up, but no provider is connected."})
		return tea.Batch(m.commitMessages()...)
	}
	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt)})
		return tea.Batch(m.commitMessages()...)
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Scheduled task fired"})
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: prompt})
	return m.sendToAgent(prompt, nil)
}

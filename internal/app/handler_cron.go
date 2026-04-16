package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/core"
)

// updateCron handles cron-related messages.
func (m *model) updateCron(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(appsystem.CronTickMsg); ok {
		return m.handleCronTick(), true
	}
	return nil, false
}

// handleCronTick checks for due jobs and fires them if the REPL is idle.
func (m *model) handleCronTick() tea.Cmd {
	idle := !m.conv.Stream.Active && !m.isToolPhaseActive()
	result := m.systemInput.HandleCronTick(idle)

	cmds := []tea.Cmd{appsystem.StartCronTicker()}

	if result.InjectPrompt != "" {
		cmds = append(cmds, m.injectCronPrompt(result.InjectPrompt))
	}
	for _, notice := range result.Notices {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: notice,
		})
	}

	return tea.Batch(cmds...)
}

// injectCronPrompt injects a cron prompt as a user message and starts an LLM stream.
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

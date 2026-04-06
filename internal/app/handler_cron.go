package app

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/message"
)

const cronTickInterval = 30 * time.Second

// cronTickMsg is sent periodically to check for due cron jobs.
type cronTickMsg struct{}

// triggerCronTickNow returns a command that immediately checks cron jobs once.
func triggerCronTickNow() tea.Cmd {
	return func() tea.Msg { return cronTickMsg{} }
}

// startCronTicker returns a command that sends the first cronTickMsg.
func startCronTicker() tea.Cmd {
	return tea.Tick(cronTickInterval, func(time.Time) tea.Msg {
		return cronTickMsg{}
	})
}

// updateCron handles cron-related messages.
func (m *model) updateCron(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(cronTickMsg); ok {
		return m.handleCronTick(), true
	}
	return nil, false
}

// handleCronTick checks for due jobs and fires them if the REPL is idle.
func (m *model) handleCronTick() tea.Cmd {
	// Skip lock acquisition and iteration when no jobs exist
	if cron.DefaultStore.Empty() && len(m.cronQueue) == 0 {
		return startCronTicker()
	}

	fired := cron.DefaultStore.Tick()

	cmds := []tea.Cmd{startCronTicker()}
	idle := !m.conv.Stream.Active && !m.hasPendingToolExecution()

	for _, f := range fired {
		if !idle {
			m.cronQueue = append(m.cronQueue, f.Prompt)
		} else {
			cmds = append(cmds, m.injectCronPrompt(f.Prompt))
		}
	}

	// Drain one queued prompt if idle
	if idle {
		if cmd := m.drainCronQueue(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

// drainCronQueue pops and injects one queued cron prompt. Returns nil if the queue is empty.
func (m *model) drainCronQueue() tea.Cmd {
	if len(m.cronQueue) == 0 {
		return nil
	}
	prompt := m.cronQueue[0]
	m.cronQueue = m.cronQueue[1:]
	return m.injectCronPrompt(prompt)
}

// injectCronPrompt injects a cron prompt as a user message and starts an LLM stream.
func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.provider.LLM == nil {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.conv.Append(message.ChatMessage{
		Role:    message.RoleNotice,
		Content: "Scheduled task fired",
	})
	m.conv.Append(message.ChatMessage{
		Role:    message.RoleUser,
		Content: prompt,
	})

	return m.startLLMStream(nil)
}

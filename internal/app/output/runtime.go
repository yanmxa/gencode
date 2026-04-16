package output

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/ui/progress"
	"github.com/yanmxa/gencode/internal/core"
)

// AgentOutboxMsg carries an event from the core.Agent outbox to the TUI.
type AgentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// Runtime defines the callbacks needed to process the agent outbox path.
type Runtime interface {
	HandleAgentEvent(core.Event) tea.Cmd
	HandleAgentStopped(error) tea.Cmd
	HandleProgress(progress.UpdateMsg) tea.Cmd
	HandleProgressTick() tea.Cmd
}

// DrainAgentOutbox blocks until the next outbox event arrives, then emits an AgentOutboxMsg.
func DrainAgentOutbox(outbox <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-outbox
		if !ok {
			return AgentOutboxMsg{Closed: true}
		}
		return AgentOutboxMsg{Event: ev}
	}
}

// Update routes agent outbox and progress messages for the output path.
func Update(rt Runtime, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			return rt.HandleAgentStopped(nil), true
		}
		return rt.HandleAgentEvent(msg.Event), true
	case progress.UpdateMsg:
		return rt.HandleProgress(msg), true
	case progress.CheckTickMsg:
		return rt.HandleProgressTick(), true
	default:
		return nil, false
	}
}

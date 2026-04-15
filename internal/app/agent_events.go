package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
)

// agentOutboxMsg carries an event from the core.Agent's outbox to the TUI.
type agentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// drainAgentOutbox returns a tea.Cmd that reads events from the agent's outbox
// channel and emits them as agentOutboxMsg for the TUI.
func drainAgentOutbox(outbox <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-outbox
		if !ok {
			return agentOutboxMsg{Closed: true}
		}
		return agentOutboxMsg{Event: ev}
	}
}

// legacyMessageToCore converts a legacy message.Message to core.Message.
func legacyMessageToCore(m message.Message) core.Message {
	return message.ToCore(m)
}

// updateAgent handles core.Agent events (outbox events and permission bridge
// requests) in the TUI update loop.
func (m *model) updateAgent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case agentOutboxMsg:
		if msg.Closed {
			return nil, true
		}
		return m.handleAgentEvent(msg.Event), true
	case agentPermissionMsg:
		m.pendingPermBridge = msg.Request
		return m.showPermissionPrompt(msg.Request), true
	}
	return nil, false
}

// handleAgentEvent processes a single event from the core.Agent outbox.
func (m *model) handleAgentEvent(ev core.Event) tea.Cmd {
	switch ev.Type {
	case core.OnChunk:
		if chunk, ok := ev.Chunk(); ok {
			if chunk.Text != "" {
				m.conv.AppendToLast(chunk.Text, "")
			}
		}
		return drainAgentOutbox(m.agentSess.agent.Outbox())
	case core.OnTurn:
		return nil
	case core.PreTool:
		return drainAgentOutbox(m.agentSess.agent.Outbox())
	case core.PostTool:
		return drainAgentOutbox(m.agentSess.agent.Outbox())
	default:
		return drainAgentOutbox(m.agentSess.agent.Outbox())
	}
}

// showPermissionPrompt shows the TUI permission approval dialog for a bridge request.
func (m *model) showPermissionPrompt(req *PermBridgeRequest) tea.Cmd {
	if req == nil || req.Request == nil {
		return nil
	}
	m.approval.Show(req.Request, m.width, m.height)
	return nil
}

// sendToAgent sends a user message to the core.Agent's inbox.
func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	msg := core.Message{
		Role:    core.RoleUser,
		Content: content,
		Images:  images,
	}
	return func() tea.Msg {
		m.agentSess.agent.Inbox() <- msg
		return nil
	}
}

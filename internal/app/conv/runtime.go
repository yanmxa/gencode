package conv

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
)

type AgentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// Runtime defines callbacks that the conv event handlers need from the root
// model. Each method represents a coherent operation (not a fine-grained
// primitive), keeping the interface small and each implementation substantial.
type Runtime interface {
	CommitMessages() []tea.Cmd
	ContinueOutbox() tea.Cmd
	SetTokenCounts(in, out int)
	ProcessToolResult(tr core.ToolResult) *core.ToolResult
	ProcessTurnEnd(result core.Result) tea.Cmd
	ProcessAgentStop(err error) tea.Cmd
	HandlePermBridge(req *PermBridgeRequest) tea.Cmd
	HandleCompactResult(msg CompactResultMsg) tea.Cmd
	HandleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd
	HasRunningTasks() bool
}

type PermBridgeMsg struct {
	Request *PermBridgeRequest
}

func PollPermBridge(pb *PermissionBridge) tea.Cmd {
	return func() tea.Msg {
		req, ok := pb.Recv()
		if !ok {
			return nil
		}
		return PermBridgeMsg{Request: req}
	}
}

func DrainAgentOutbox(outbox <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-outbox
		if !ok {
			return AgentOutboxMsg{Closed: true}
		}
		return AgentOutboxMsg{Event: ev}
	}
}

// Update routes all output-path messages: agent outbox, permission bridge,
// compaction results, and progress updates.
func Update(rt Runtime, m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			m.Stream.Stop()
			return rt.ProcessAgentStop(nil), true
		}
		return handleAgentEvent(rt, m, msg.Event), true
	case PermBridgeMsg:
		return rt.HandlePermBridge(msg.Request), true
	case CompactResultMsg:
		return rt.HandleCompactResult(msg), true
	case kit.TokenLimitResultMsg:
		return rt.HandleTokenLimitResult(msg), true
	case ProgressUpdateMsg:
		return m.HandleProgress(msg), true
	case ProgressCheckTickMsg:
		return m.HandleProgressTick(rt.HasRunningTasks()), true
	default:
		return nil, false
	}
}

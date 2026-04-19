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
	HandleAgentCompact(info core.CompactInfo) tea.Cmd
	HandleCompactResult(msg CompactResultMsg) tea.Cmd
	HandleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd
	HasRunningTasks() bool
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


package output

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/output/progress"
	"github.com/yanmxa/gencode/internal/core"
)

// AgentOutboxMsg carries an event from the core.Agent outbox to the TUI.
type AgentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// ConversationMutator handles message append, commit, and notice operations.
type ConversationMutator interface {
	CommitMessages() []tea.Cmd
	AppendMessage(msg core.ChatMessage)
	AppendToLast(text, thinking string)
	SetLastToolCalls(calls []core.ToolCall)
	SetLastThinkingSignature(sig string)
	AddNotice(text string)
}

// StreamController manages LLM streaming lifecycle and outbox continuation.
type StreamController interface {
	ActivateStream()
	SetBuildingTool(name string)
	StopStream()
	ContinueOutbox() tea.Cmd
}

// ToolSideEffects handles post-tool-execution side effects and hooks.
type ToolSideEffects interface {
	ApplyToolSideEffects(toolName string, sideEffect any)
	FirePostToolHook(tr core.ToolResult, sideEffect any)
	PersistOverflow(result *core.ToolResult)
}

// TurnMetrics tracks token counts, turn counters, and transient per-turn state.
type TurnMetrics interface {
	SetTokenCounts(in, out int)
	ClearWarningSuppressed()
	IncrementTurnCounter()
	ResetTurnCounter()
	ClearThinkingOverride()
}

// TurnHooks fires lifecycle hooks at turn boundaries.
type TurnHooks interface {
	FireIdleHooks() bool
	FireStopFailureHook(err error)
}

// SessionPersistence handles session saving, compaction, and agent lifecycle.
type SessionPersistence interface {
	SaveSession()
	ShouldAutoCompact() bool
	SetAutoCompactContinue()
	TriggerAutoCompact() tea.Cmd
	StopAgentSession()
}

// QueueDrainer drains pending work (input, cron, hooks, notifications)
// and starts prompt suggestions at turn boundaries.
type QueueDrainer interface {
	StartPromptSuggestion() tea.Cmd
	DrainInputQueue() tea.Cmd
	DrainCronQueue() tea.Cmd
	DrainAsyncHookQueue() tea.Cmd
	DrainTaskNotifications() tea.Cmd
}

// TurnManager is the composition of all turn-boundary interfaces.
type TurnManager interface {
	TurnMetrics
	TurnHooks
	SessionPersistence
	QueueDrainer
}

// Runtime is the union of all interfaces needed by the output event handlers.
// The parent app model satisfies all four sub-interfaces.
type Runtime interface {
	ConversationMutator
	StreamController
	ToolSideEffects
	TurnManager
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
func Update(rt Runtime, m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			return handleAgentStopped(rt, m, nil), true
		}
		return handleAgentEvent(rt, m, msg.Event), true
	case progress.UpdateMsg:
		return m.HandleProgress(msg), true
	case progress.CheckTickMsg:
		return m.HandleProgressTick(false), true
	default:
		return nil, false
	}
}

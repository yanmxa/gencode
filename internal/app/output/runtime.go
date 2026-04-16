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

// Runtime defines the callbacks the output package needs from the parent app model.
type Runtime interface {
	CommitMessages() []tea.Cmd
	AppendMessage(msg core.ChatMessage)
	AppendToLast(text, thinking string)
	SetLastToolCalls(calls []core.ToolCall)
	SetLastThinkingSignature(sig string)
	AddNotice(text string)

	ActivateStream()
	SetBuildingTool(name string)
	StopStream()

	SetTokenCounts(in, out int)
	ClearWarningSuppressed()
	IncrementTurnCounter()
	ResetTurnCounter()

	ClearThinkingOverride()

	ContinueOutbox() tea.Cmd

	ApplyToolSideEffects(toolName string, sideEffect any)
	FirePostToolHook(tr core.ToolResult, sideEffect any)
	PersistOverflow(result *core.ToolResult)

	FireIdleHooks() bool
	SaveSession()
	ShouldAutoCompact() bool
	SetAutoCompactContinue()
	TriggerAutoCompact() tea.Cmd
	StartPromptSuggestion() tea.Cmd
	DrainInputQueue() tea.Cmd
	DrainCronQueue() tea.Cmd
	DrainAsyncHookQueue() tea.Cmd
	DrainTaskNotifications() tea.Cmd

	FireStopFailureHook(err error)
	StopAgentSession()
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

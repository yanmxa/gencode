package output

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/output/compact"
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

// TurnMetrics tracks token counts and transient per-turn state.
type TurnMetrics interface {
	SetTokenCounts(in, out int)
	ClearWarningSuppressed()
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

// TurnQueueCoordinator coordinates prompt suggestion and Source 1/2/3 queue
// draining after a turn finishes.
type TurnQueueCoordinator interface {
	StartPromptSuggestion() tea.Cmd
	DrainTurnQueues() tea.Cmd
}

// ProgressRuntime coordinates task-progress polling behavior.
type ProgressRuntime interface {
	HasRunningTasks() bool
}

// TurnManager is the composition of all turn-boundary interfaces.
type TurnManager interface {
	TurnMetrics
	TurnHooks
	SessionPersistence
	TurnQueueCoordinator
}

// CompactHandler handles compaction and token limit result processing.
type CompactHandler interface {
	HandleCompactResult(msg compact.ResultMsg) tea.Cmd
	HandleTokenLimitResult(msg compact.TokenLimitResultMsg) tea.Cmd
}

// PermBridgeHandler handles permission bridge events from the agent.
type PermBridgeHandler interface {
	StorePendingPermRequest(req *PermBridgeRequest)
	ShowPermissionPrompt(req *PermBridgeRequest) tea.Cmd
}

// Runtime is the union of all interfaces needed by the output event handlers.
type Runtime interface {
	ConversationMutator
	StreamController
	ToolSideEffects
	TurnManager
	CompactHandler
	PermBridgeHandler
	ProgressRuntime
}

// PermBridgeMsg carries a permission bridge request from the agent to the TUI.
type PermBridgeMsg struct {
	Request *PermBridgeRequest
}

// PollPermBridge blocks until the next permission request arrives.
func PollPermBridge(pb *PermissionBridge) tea.Cmd {
	return func() tea.Msg {
		req, ok := pb.Recv()
		if !ok {
			return nil
		}
		return PermBridgeMsg{Request: req}
	}
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

// Update routes all output-path messages: agent outbox, permission bridge,
// compaction results, and progress updates.
func Update(rt Runtime, m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			return handleAgentStopped(rt, m, nil), true
		}
		return handleAgentEvent(rt, m, msg.Event), true
	case PermBridgeMsg:
		rt.StorePendingPermRequest(msg.Request)
		return rt.ShowPermissionPrompt(msg.Request), true
	case compact.ResultMsg:
		return rt.HandleCompactResult(msg), true
	case compact.TokenLimitResultMsg:
		return rt.HandleTokenLimitResult(msg), true
	case progress.UpdateMsg:
		return m.HandleProgress(msg), true
	case progress.CheckTickMsg:
		return m.HandleProgressTick(rt.HasRunningTasks()), true
	default:
		return nil, false
	}
}

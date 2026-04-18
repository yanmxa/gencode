package conv

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
)

// AgentOutboxMsg carries an event from the core.Agent outbox to the TUI.
type AgentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// Runtime defines callbacks that the conv event handlers need from the root
// model. State mutations on ConversationModel are done directly (passed as a
// separate parameter to Update/handlers); this interface covers only operations
// that require resources outside the conv package.
//
// Composed from six sub-interfaces, each grouping a logical concern:
//   - MessageRuntime:     message commit and agent outbox
//   - TokenRuntime:       token counts and thinking state
//   - ToolEffectRuntime:  tool side effects (cwd, file cache, hooks)
//   - TurnRuntime:        turn lifecycle, session, auto-compact, queue drain
//   - PermissionRuntime:  permission bridge requests
//   - ProgressRuntime:    background task progress
type Runtime interface {
	MessageRuntime
	TokenRuntime
	ToolEffectRuntime
	TurnRuntime
	PermissionRuntime
	ProgressRuntime
}

// MessageRuntime handles message commit and agent outbox continuation.
type MessageRuntime interface {
	CommitMessages() []tea.Cmd
	ContinueOutbox() tea.Cmd
}

// TokenRuntime handles token counts and thinking state reset.
type TokenRuntime interface {
	SetTokenCounts(in, out int)
	ClearThinkingOverride()
}

// ToolEffectRuntime handles tool execution side effects: cwd changes,
// file cache updates, post-tool hooks, and overflow persistence.
type ToolEffectRuntime interface {
	PopToolSideEffect(toolCallID string) any
	ApplyToolSideEffects(toolName string, sideEffect any)
	FirePostToolHook(tr core.ToolResult, sideEffect any)
	PersistOverflow(result *core.ToolResult)
}

// TurnRuntime handles turn lifecycle: idle hooks, session persistence,
// auto-compact, agent session stop, and turn queue drain.
type TurnRuntime interface {
	FireIdleHooks() bool
	FireStopFailureHook(err error)
	SaveSession()
	ShouldAutoCompact() bool
	TriggerAutoCompact() tea.Cmd
	StopAgentSession()
	StartPromptSuggestion() tea.Cmd
	DrainTurnQueues() tea.Cmd
	HandleCompactResult(msg CompactResultMsg) tea.Cmd
	HandleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd
}

// PermissionRuntime handles permission bridge request forwarding.
type PermissionRuntime interface {
	StorePendingPermRequest(req *PermBridgeRequest)
	ShowPermissionPrompt(req *PermBridgeRequest) tea.Cmd
}

// ProgressRuntime reports background task status.
type ProgressRuntime interface {
	HasRunningTasks() bool
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
// compaction results, and progress updates. The ConversationModel is passed
// directly so handlers can mutate conversation state without forwarding methods.
func Update(rt Runtime, m *OutputModel, cm *ConversationModel, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			return handleAgentStopped(rt, m, cm, nil), true
		}
		return handleAgentEvent(rt, m, cm, msg.Event), true
	case PermBridgeMsg:
		rt.StorePendingPermRequest(msg.Request)
		return rt.ShowPermissionPrompt(msg.Request), true
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

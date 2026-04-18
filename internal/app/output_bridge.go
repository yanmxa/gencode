package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

const autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."
const minMessagesForCompaction = 3

type permissionDecision struct {
	Approved bool
	AllowAll bool
	Request  *perm.PermissionRequest
}

var _ conv.Runtime = (*model)(nil)

func (m *model) CommitMessages() []tea.Cmd              { return m.commitMessages() }
func (m *model) AppendMessage(msg core.ChatMessage)     { m.conv.Append(msg) }
func (m *model) AppendToLast(text, thinking string)     { m.conv.AppendToLast(text, thinking) }
func (m *model) SetLastToolCalls(calls []core.ToolCall) { m.conv.SetLastToolCalls(calls) }
func (m *model) SetLastThinkingSignature(sig string)    { m.conv.SetLastThinkingSignature(sig) }
func (m *model) AddNotice(text string)                  { m.conv.AddNotice(text) }
func (m *model) ActivateStream() {
	m.conv.Stream.Active = true
	m.conv.Stream.BuildingTool = ""
}
func (m *model) SetBuildingTool(name string) { m.conv.Stream.BuildingTool = name }
func (m *model) StopStream()                 { m.conv.Stream.Stop() }
func (m *model) SetTokenCounts(in, out int) {
	m.runtime.InputTokens = in
	m.runtime.OutputTokens = out
}
func (m *model) ClearWarningSuppressed()                 { m.conv.Compact.WarningSuppressed = false }
func (m *model) ClearThinkingOverride()                  { m.runtime.ThinkingOverride = llm.ThinkingOff }
func (m *model) ContinueOutbox() tea.Cmd                 { return m.continueOutbox() }
func (m *model) PopToolSideEffect(toolCallID string) any { return tool.PopSideEffect(toolCallID) }
func (m *model) ApplyToolSideEffects(toolName string, sideEffect any) {
	m.applyAgentToolSideEffects(toolName, sideEffect)
}
func (m *model) FirePostToolHook(tr core.ToolResult, sideEffect any) {
	m.runtime.FirePostToolHook(tr, sideEffect)
}
func (m *model) PersistOverflow(result *core.ToolResult) { m.persistToolResultOverflow(result) }
func (m *model) FireIdleHooks() bool                     { return m.fireIdleHooks() }
func (m *model) SaveSession() {
	if err := m.saveSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}
}
func (m *model) ShouldAutoCompact() bool     { return m.shouldAutoCompact() }
func (m *model) SetAutoCompactContinue()     { m.conv.Compact.AutoContinue = true }
func (m *model) TriggerAutoCompact() tea.Cmd { return m.triggerAutoCompact() }
func (m *model) StartPromptSuggestion() tea.Cmd {
	return m.startPromptSuggestion()
}
func (m *model) DrainTurnQueues() tea.Cmd {
	for _, drain := range []func() tea.Cmd{m.drainInputQueueToAgent, m.drainCronQueueToAgent, m.drainAsyncHookQueueToAgent, m.drainTaskNotificationsToAgent} {
		if cmd := drain(); cmd != nil {
			return cmd
		}
	}
	return nil
}
func (m *model) HasRunningTasks() bool { return tracker.DefaultStore.HasInProgress() }
func (m *model) FireStopFailureHook(err error) {
	m.runtime.FireStopFailureHook(core.LastAssistantChatContent(m.conv.Messages), err)
}
func (m *model) StopAgentSession() {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
}
func (m *model) HandleCompactResult(msg conv.CompactResultMsg) tea.Cmd {
	return m.handleCompactResult(msg)
}
func (m *model) HandleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd {
	return m.handleTokenLimitResult(msg)
}
func (m *model) StorePendingPermRequest(req *conv.PermBridgeRequest) {
	if m.agentSess != nil {
		m.agentSess.pendingPermRequest = req
	}
}
func (m *model) ShowPermissionPrompt(req *conv.PermBridgeRequest) tea.Cmd {
	if req == nil {
		return nil
	}
	m.userInput.Approval.Show(&perm.PermissionRequest{ToolName: req.ToolName, Description: req.Description}, m.width, m.height)
	return nil
}

func (m *model) handlePermBridgeDecision(decision permissionDecision) tea.Cmd {
	if m.agentSess == nil {
		return nil
	}
	req := m.agentSess.pendingPermRequest
	m.agentSess.pendingPermRequest = nil
	if req == nil {
		return nil
	}
	resp := conv.PermBridgeResponse{Allow: decision.Approved, Reason: "user decision"}
	if decision.Approved {
		if decision.AllowAll && m.runtime.SessionPermissions != nil && decision.Request != nil {
			m.runtime.SessionPermissions.AllowTool(decision.Request.ToolName)
		}
		resp.Reason = "user approved"
	} else {
		resp.Reason = "user denied"
	}
	select {
	case req.Response <- resp:
	default:
	}
	return conv.PollPermBridge(m.agentSess.permBridge)
}

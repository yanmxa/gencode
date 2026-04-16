// output.Runtime implementation for the Agent Output path.
// Handler logic lives in internal/app/output/update.go; this file provides
// the mutation primitives that those handlers call via the Runtime interface.
// Permission bridge routing lives in update_output_perm_bridge.go.
package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/util/log"
)

// --- Dispatcher ---

// updateOutput handles core.Agent events (outbox events and permission bridge
// requests) in the TUI update loop.
func (m *model) updateOutput(msg tea.Msg) (tea.Cmd, bool) {
	if msg, ok := msg.(agentPermissionMsg); ok {
		if m.agentSess != nil {
			m.agentSess.pendingPermRequest = msg.Request
		}
		return m.showPermissionPrompt(msg.Request), true
	}
	return appoutput.Update(m, &m.agentOutput, msg)
}

// Compile-time checks: *model satisfies each sub-interface of output.Runtime.
var (
	_ appoutput.ConversationMutator = (*model)(nil)
	_ appoutput.StreamController    = (*model)(nil)
	_ appoutput.ToolSideEffects     = (*model)(nil)
	_ appoutput.TurnManager         = (*model)(nil)
)

// --- output.Runtime interface implementation ---

func (m *model) CommitMessages() []tea.Cmd             { return m.commitMessages() }
func (m *model) AppendMessage(msg core.ChatMessage)    { m.conv.Append(msg) }
func (m *model) AppendToLast(text, thinking string)    { m.conv.AppendToLast(text, thinking) }
func (m *model) SetLastToolCalls(calls []core.ToolCall) { m.conv.SetLastToolCalls(calls) }
func (m *model) SetLastThinkingSignature(sig string)   { m.conv.SetLastThinkingSignature(sig) }
func (m *model) AddNotice(text string)                 { m.conv.AddNotice(text) }

func (m *model) ActivateStream() {
	m.conv.Stream.Active = true
	m.conv.Stream.BuildingTool = ""
}
func (m *model) SetBuildingTool(name string) { m.conv.Stream.BuildingTool = name }
func (m *model) StopStream()                 { m.conv.Stream.Stop() }

func (m *model) SetTokenCounts(in, out int) {
	m.inputTokens = in
	m.outputTokens = out
}
func (m *model) ClearWarningSuppressed() { m.conv.Compact.WarningSuppressed = false }
func (m *model) IncrementTurnCounter()   { m.conv.TurnsSinceLastTaskTool++ }
func (m *model) ResetTurnCounter()       { m.conv.TurnsSinceLastTaskTool = 0 }
func (m *model) ClearThinkingOverride()  { m.thinkingOverride = provider.ThinkingOff }
func (m *model) ContinueOutbox() tea.Cmd { return m.continueOutbox() }

func (m *model) ApplyToolSideEffects(toolName string, sideEffect any) {
	m.applyAgentToolSideEffects(toolName, sideEffect)
}
func (m *model) FirePostToolHook(tr core.ToolResult, sideEffect any) {
	m.fireAgentPostToolHook(tr, sideEffect)
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
func (m *model) DrainInputQueue() tea.Cmd        { return m.drainInputQueueToAgent() }
func (m *model) DrainCronQueue() tea.Cmd         { return m.drainCronQueueToAgent() }
func (m *model) DrainAsyncHookQueue() tea.Cmd    { return m.drainAsyncHookQueueToAgent() }
func (m *model) DrainTaskNotifications() tea.Cmd { return m.drainTaskNotificationsToAgent() }

func (m *model) FireStopFailureHook(err error) {
	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.StopFailure, hooks.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			Error:                err.Error(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
	}
}

func (m *model) StopAgentSession() {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
}

// --- Side effects (parent-level, needs model internals) ---

func (m *model) applyAgentToolSideEffects(toolName string, sideEffect any) {
	resp, ok := sideEffect.(map[string]any)
	if !ok {
		return
	}

	m.syncBackgroundTaskTrackerFromAgent(toolName, resp)

	switch toolName {
	case "Bash":
		if newCwd := getHookResponseString(resp, "cwd"); newCwd != "" {
			m.changeCwd(newCwd)
		}
	case tool.ToolEnterWorktree:
		if worktreePath := getHookResponseString(resp, "worktreePath"); worktreePath != "" {
			m.changeCwd(worktreePath)
		}
	case tool.ToolExitWorktree:
		if restoredPath := getHookResponseString(resp, "restoredPath"); restoredPath != "" {
			m.changeCwd(restoredPath)
		}
	case "Write", "Edit":
		if filePath := getHookResponseString(resp, "filePath"); filePath != "" {
			m.fireFileChanged(filePath, toolName)
			if m.fileCache != nil {
				m.fileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := getHookResponseString(fileData, "filePath"); filePath != "" {
				if m.fileCache != nil {
					m.fileCache.Touch(filePath)
				}
			}
		}
	}
}

func (m *model) syncBackgroundTaskTrackerFromAgent(toolName string, resp map[string]any) {
	if !tool.IsAgentToolName(toolName) {
		return
	}
	bg, ok := resp["backgroundTask"].(map[string]any)
	if !ok {
		return
	}
	launch := appagent.BackgroundTaskLaunch{
		TaskID:      appagent.MetadataString(bg, "taskId"),
		AgentName:   appagent.MetadataString(bg, "agentName"),
		AgentType:   appagent.MetadataString(bg, "agentType"),
		Description: appagent.MetadataString(bg, "description"),
		ResumeID:    appagent.MetadataString(bg, "resumeId"),
	}
	if launch.TaskID == "" {
		return
	}
	childID := appagent.EnsureBackgroundWorkerTracker(launch, "", "")
	if childID != "" {
		appagent.RecordBackgroundTaskLaunch(launch, "", "", 0)
	}
}

func (m *model) fireAgentPostToolHook(tr core.ToolResult, sideEffect any) {
	if m.hookEngine == nil {
		return
	}
	eventType := hooks.PostToolUse
	if tr.IsError {
		eventType = hooks.PostToolUseFailure
	}
	toolResponse := any(tr.Content)
	if sideEffect != nil {
		toolResponse = sideEffect
	}
	input := hooks.HookInput{
		ToolName:     tr.ToolName,
		ToolUseID:    tr.ToolCallID,
		ToolResponse: toolResponse,
	}
	if tr.IsError {
		input.Error = tr.Content
	}
	m.hookEngine.ExecuteAsync(eventType, input)
}

func (m *model) fireIdleHooks() bool {
	if m.hookEngine == nil {
		return false
	}

	blocked := false
	if m.hookEngine.HasHooks(hooks.Stop) {
		outcome := m.hookEngine.Execute(context.Background(), hooks.Stop, hooks.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
		if outcome.ShouldBlock {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleUser,
				Content: "Stop hook blocked: " + outcome.BlockReason,
			})
			if m.agentSess != nil {
				m.agentSess.agent.Inbox() <- core.Message{
					Role:    core.RoleUser,
					Content: "Stop hook blocked: " + outcome.BlockReason,
				}
			}
			blocked = true
		}
	}

	m.hookEngine.ExecuteAsync(hooks.Notification, hooks.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
	return blocked
}

// --- Continuation injection helpers ---

func (m *model) drainInputQueueToAgent() tea.Cmd {
	if m.inputQueue.Len() == 0 {
		return nil
	}
	item, ok := m.inputQueue.Dequeue()
	if !ok {
		return nil
	}
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: item.Content,
		Images:  item.Images,
	})
	return m.sendToAgent(item.Content, item.Images)
}

func (m *model) drainCronQueueToAgent() tea.Cmd {
	if len(m.systemInput.CronQueue) == 0 {
		return nil
	}
	prompt := m.systemInput.CronQueue[0]
	m.systemInput.CronQueue = m.systemInput.CronQueue[1:]

	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Scheduled task fired"})
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: prompt})
	return m.sendToAgent(prompt, nil)
}

func (m *model) drainAsyncHookQueueToAgent() tea.Cmd {
	if m.systemInput.AsyncHookQueue == nil {
		return nil
	}
	item, ok := m.systemInput.AsyncHookQueue.Pop()
	if !ok {
		return nil
	}

	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if len(item.Context) == 0 && item.ContinuationPrompt == "" {
		return nil
	}

	var content string
	if item.ContinuationPrompt != "" {
		content = item.ContinuationPrompt
	}
	for _, ctx := range item.Context {
		if content != "" {
			content += "\n"
		}
		content += ctx
	}

	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: content})
	return m.sendToAgent(content, nil)
}

func (m *model) drainTaskNotificationsToAgent() tea.Cmd {
	if m.agentInput.Notifications == nil {
		return nil
	}
	items := appagent.PopReadyNotifications(m.agentInput.Notifications, true)
	if len(items) == 0 {
		return nil
	}
	return m.injectTaskNotificationContinuation(appagent.MergeNotifications(items))
}

func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	inbox := m.agentSess.agent.Inbox()
	msg := core.Message{
		Role:    core.RoleUser,
		Content: content,
		Images:  images,
	}
	return func() tea.Msg {
		inbox <- msg
		return nil
	}
}

func (m *model) continueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return appoutput.DrainAgentOutbox(m.agentSess.agent.Outbox())
}

// --- Overflow persistence ---

const (
	toolResultOverflowThreshold = 100_000
	toolResultPreviewSize       = 10_000
)

func (m *model) persistToolResultOverflow(result *core.ToolResult) {
	if len(result.Content) <= toolResultOverflowThreshold {
		return
	}

	cutoff := toolResultPreviewSize
	if cutoff > len(result.Content) {
		cutoff = len(result.Content)
	}
	for cutoff > 0 && !utf8.RuneStart(result.Content[cutoff]) {
		cutoff--
	}
	preview := result.Content[:cutoff]

	persisted := false
	if err := m.ensureSessionStore(); err == nil && m.sessionID != "" {
		if err := m.sessionStore.PersistToolResult(m.sessionID, result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}

	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.sessionID, result.ToolCallID)
	} else {
		result.Content = fmt.Sprintf("%s\n\n[Output truncated from %d bytes — full content not persisted]", preview, len(result.Content))
	}
}

func getHookResponseString(resp map[string]any, key string) string {
	if value, ok := resp[key].(string); ok {
		return value
	}
	return ""
}

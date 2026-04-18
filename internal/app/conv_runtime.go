package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
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
func (m *model) ShouldAutoCompact() bool {
	return kit.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}
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
			if m.runtime.FileCache != nil {
				m.runtime.FileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := getHookResponseString(fileData, "filePath"); filePath != "" && m.runtime.FileCache != nil {
				m.runtime.FileCache.Touch(filePath)
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
	launch := notify.BackgroundTaskLaunch{
		TaskID:      notify.MetadataString(bg, "taskId"),
		AgentName:   notify.MetadataString(bg, "agentName"),
		AgentType:   notify.MetadataString(bg, "agentType"),
		Description: notify.MetadataString(bg, "description"),
		ResumeID:    notify.MetadataString(bg, "resumeId"),
	}
	if launch.TaskID == "" {
		return
	}
	childID := notify.EnsureBackgroundWorkerTracker(launch, "", "")
	if childID != "" {
		notify.RecordBackgroundTaskLaunch(launch, "", "", 0)
	}
}

func (m *model) fireIdleHooks() bool {
	lastContent := core.LastAssistantChatContent(m.conv.Messages)
	blocked, reason := m.runtime.ExecuteIdleHooks(context.Background(), lastContent)
	if blocked {
		msg := "Stop hook blocked: " + reason
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: msg})
		if m.agentSess != nil {
			m.agentSess.agent.Inbox() <- core.Message{Role: core.RoleUser, Content: msg}
		}
	}
	return blocked
}

func (m *model) drainInputQueueToAgent() tea.Cmd {
	if m.userInput.Queue.Len() == 0 {
		return nil
	}
	item, ok := m.userInput.Queue.Dequeue()
	if !ok {
		return nil
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: item.Content, Images: item.Images})
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
	items := notify.PopReadyNotifications(m.agentInput.Notifications, true)
	if len(items) == 0 {
		return nil
	}
	return m.InjectTaskNotificationContinuation(notify.MergeNotifications(items))
}

func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	inbox := m.agentSess.agent.Inbox()
	msg := core.Message{Role: core.RoleUser, Content: content, Images: images}
	return func() tea.Msg {
		inbox <- msg
		return nil
	}
}

func (m *model) continueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return conv.DrainAgentOutbox(m.agentSess.agent.Outbox())
}

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
	if err := m.runtime.EnsureSessionStore(m.cwd); err == nil && m.runtime.SessionID != "" {
		if err := m.runtime.SessionStore.PersistToolResult(m.runtime.SessionID, result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}
	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.runtime.SessionID, result.ToolCallID)
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

func (m *model) triggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = conv.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", kit.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.agentOutput.Spinner.Tick, conv.CompactCmd(m.buildCompactRequest("", "auto")))
	return tea.Batch(commitCmds...)
}

func (m *model) handleCompactResult(msg conv.CompactResultMsg) tea.Cmd {
	shouldContinue := m.conv.Compact.AutoContinue
	if msg.Error != nil {
		m.conv.Compact.Complete(fmt.Sprintf("Compaction could not be completed: %v", msg.Error), true)
		return tea.Batch(m.commitMessages()...)
	}
	m.conv.Compact.Complete(fmt.Sprintf("Condensed %d earlier messages.", msg.OriginalCount), false)
	scrollbackCmds := m.commitAllMessages()
	boundaryStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	boundary := boundaryStyle.Render(fmt.Sprintf("✻ Conversation compacted — %d messages summarized (scroll up for history)", msg.OriginalCount))
	m.resetAfterCompact()
	var restoredFiles []filecache.RestoredFile
	var restoredContext string
	if m.runtime.FileCache != nil {
		restoredFiles, _ = m.runtime.FileCache.RestoreRecent()
		if len(restoredFiles) > 0 {
			restoredContext = filecache.FormatRestoredFiles(restoredFiles)
		}
	}
	if m.runtime.SessionStore != nil && m.runtime.SessionID != "" {
		_ = m.runtime.SessionStore.SaveSessionMemory(m.runtime.SessionID, msg.Summary)
	}
	m.runtime.SessionSummary = msg.Summary
	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.ExecuteAsync(hook.PostCompact, hook.HookInput{Trigger: msg.Trigger})
	}
	scrollPart := tea.Sequence(append(scrollbackCmds, tea.Println(boundary), tea.ClearScreen)...)
	cmds := []tea.Cmd{scrollPart}
	if shouldContinue {
		m.conv.Compact.ClearResult()
		if restoredContext != "" {
			m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: restoredContext})
		}
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: autoCompactResumePrompt})
		cmds = append(cmds, m.sendToAgent(autoCompactResumePrompt, nil))
	} else if restoredContext != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: restoredContext})
		m.conv.AddNotice(fmt.Sprintf("Restored %d recently accessed file(s) for context.", len(restoredFiles)))
		cmds = append(cmds, m.commitMessages()...)
	}
	return tea.Batch(cmds...)
}

func (m *model) resetAfterCompact() {
	m.conv.Clear()
	m.runtime.ResetTokens()
}

func (m *model) handleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd {
	m.userInput.Provider.FetchingLimits = false
	var content string
	if msg.Error != nil {
		content = "Error: " + msg.Error.Error()
	} else {
		content = msg.Result
	}
	m.conv.AddNotice(content)
	return tea.Batch(m.commitMessages()...)
}

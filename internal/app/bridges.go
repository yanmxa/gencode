// Runtime interface implementations: bridges between the app model and
// sub-package handlers (user overlays, agent notifications, system events).
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// --- User overlay dispatcher (Source 1) ---

func (m *model) updateUserOverlays(msg tea.Msg) (tea.Cmd, bool) {
	return input.Update(m, &m.userInput, msg)
}

// --- User overlay Runtime interface implementations ---

// user.PluginRuntime
func (m *model) GetCwd() string                { return m.cwd }
func (m *model) ReloadPluginBackedState() error { return m.reloadPluginBackedState() }

// memory.Runtime
func (m *model) ClearCachedInstructions()            { m.runtime.ClearCachedInstructions() }
func (m *model) RefreshMemoryContext(trigger string) { m.runtime.RefreshMemoryContext(m.cwd, trigger) }
func (m *model) FireFileChanged(path, tool string)   { m.fireFileChanged(path, tool) }

// user.MCPRuntime
func (m *model) SetInputText(text string) { m.userInput.Textarea.SetValue(text) }

// input.ProviderRuntime
func (m *model) SwitchProvider(p llm.Provider) {
	m.runtime.SwitchProvider(p)
	m.reconfigureAgentTool()
}
func (m *model) SetCurrentModel(cm *llm.CurrentModelInfo) { m.runtime.CurrentModel = cm }

// user.SessionRuntime
func (m *model) LoadSession(id string) error { return m.loadSession(id) }
func (m *model) ResetCommitIndex()           { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd { return m.commitAllMessages() }

// searchui.Runtime
func (m *model) SetProviderStatusMessage(msg string) { m.userInput.Provider.SetStatusMessage(msg) }

func startExternalEditor(filePath string) tea.Cmd {
	return kit.StartExternalEditor(filePath, func(err error) tea.Msg {
		return input.MemoryEditorFinishedMsg{Err: err}
	})
}

// --- User input helpers ---

func (m *model) handleStreamCancel() tea.Cmd {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	m.runtime.ClearThinkingOverride()
	m.cancelPendingToolCalls()
	m.conv.MarkLastInterrupted()

	cmds := m.commitMessages()
	if cmd := m.drainInputQueue(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m *model) cancelPendingToolCalls() {
	toolCalls := m.tool.DrainPendingCalls()
	if toolCalls == nil && len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		if lastMsg.Role == core.RoleAssistant {
			toolCalls = lastMsg.ToolCalls
		}
	}
	m.conv.AppendCancelledToolResults(toolCalls, pendingToolCancellationContent)
}

func (m *model) cancelRemainingToolCalls(startIdx int) {
	m.conv.AppendCancelledToolResults(m.tool.RemainingCalls(startIdx), func(core.ToolCall) string {
		return "Tool execution skipped."
	})
}

func pendingToolCancellationContent(tc core.ToolCall) string {
	switch tc.Name {
	case "TaskOutput":
		return "Stopped waiting for background task output because the user sent a new message. The background task may still be running."
	default:
		return "Tool execution interrupted because the user sent a new message."
	}
}

func (m *model) handleSkillInvocation() tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.AddNotice("No provider connected. Use /provider to connect.")
		m.userInput.Skill.ClearPending()
		return tea.Batch(m.commitMessages()...)
	}
	userMsg := m.userInput.Skill.ConsumeInvocation()
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})
	return m.sendToAgent(userMsg, nil)
}

func (m *model) pasteImageFromClipboard() (tea.Cmd, bool) {
	imgData, err := image.ReadImageToProviderData()
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Image paste error: " + err.Error()})
		return tea.Batch(m.commitMessages()...), true
	}
	if imgData == nil {
		return nil, false
	}
	label := m.userInput.AddPendingImage(*imgData)
	m.userInput.Images.Selection = input.ImageSelection{}
	m.userInput.Textarea.InsertString(label)
	m.userInput.UpdateHeight()
	return nil, true
}

func (m *model) quitWithCancel() (tea.Cmd, bool) {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	if m.tool.Cancel != nil {
		m.tool.Cancel()
	}
	m.fireSessionEnd("prompt_input_exit")
	return tea.Quit, true
}

// --- Agent input bridge (Source 2) ---

type agentRuntime struct {
	m *model
}

func (m *model) updateAgentInput(msg tea.Msg) (tea.Cmd, bool) {
	return notify.Update(agentRuntime{m: m}, &m.agentInput, msg)
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmd, _ := notify.Update(agentRuntime{m: m}, &m.agentInput, notify.TickMsg{})
	return cmd
}

func (rt agentRuntime) IsInputIdle() bool    { return rt.m.isInputIdle() }
func (rt agentRuntime) StreamActive() bool   { return rt.m.conv.Stream.Active }

func (rt agentRuntime) InjectTaskNotificationContinuation(item notify.Notification) tea.Cmd {
	return rt.m.injectTaskNotificationContinuation(item)
}

func (m *model) injectTaskNotificationContinuation(item notify.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if m.runtime.LLMProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleNotice,
				Content: "A background task completed, but no provider is connected.",
			})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "A background task completed, but no task notification payload was available.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	for _, ctx := range notify.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(notify.BuildContinuationPrompt(item), nil)
}

// --- System input bridge (Source 3) ---

type systemRuntime struct {
	m *model
}

func (m *model) updateSystemInput(msg tea.Msg) (tea.Cmd, bool) {
	return trigger.Update(systemRuntime{m: m}, &m.systemInput, msg)
}

func (m *model) isInputIdle() bool {
	return !m.conv.Stream.Active && !m.isToolPhaseActive()
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmd, _ := trigger.Update(systemRuntime{m: m}, &m.systemInput, trigger.AsyncHookTickMsg{})
	return cmd
}

func (rt systemRuntime) IsInputIdle() bool { return rt.m.isInputIdle() }

func (rt systemRuntime) InjectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	return rt.m.injectAsyncHookContinuation(item)
}

func (rt systemRuntime) InjectCronPrompt(prompt string) tea.Cmd {
	return rt.m.injectCronPrompt(prompt)
}

func (rt systemRuntime) AppendNotice(text string) {
	if text == "" {
		return
	}
	rt.m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: text,
	})
}

func (m *model) injectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Async hook requested a follow-up, but no provider is connected.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: "Scheduled task fired",
	})
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: prompt,
	})

	return m.sendToAgent(prompt, nil)
}

// --- Lifecycle helpers (file changes, cwd changes, project reload, hook outcomes) ---

func (m *model) fireFileChanged(filePath, source string) {
	if m.runtime.HookEngine == nil || filePath == "" {
		return
	}
	outcome := m.runtime.HookEngine.Execute(context.Background(), hook.FileChanged, hook.HookInput{
		FilePath: filePath,
		Source:   source,
		Event:    "change",
	})
	m.applyRuntimeHookOutcome(outcome)
}

func (m *model) changeCwd(newCwd string) {
	if newCwd == "" || newCwd == m.cwd {
		return
	}

	oldCwd := m.cwd
	m.cwd = newCwd
	m.isGit = setting.IsGitRepo(newCwd)
	m.userInput.Suggestions.SetCwd(newCwd)
	if m.userInput.Suggestions.GetSuggestionType() == suggest.TypeFile {
		m.userInput.Suggestions.Hide()
	}

	m.runtime.ClearCachedInstructions()
	m.runtime.RefreshMemoryContext(newCwd, "cwd_changed")
	m.reloadProjectContext(newCwd)
	m.reconfigureAgentTool()

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetCwd(newCwd)
		outcome := m.runtime.HookEngine.Execute(context.Background(), hook.CwdChanged, hook.HookInput{
			OldCwd: oldCwd,
			NewCwd: newCwd,
		})
		m.applyRuntimeHookOutcome(outcome)
	}
}

func (m *model) reloadProjectContext(cwd string) {
	initExtensions(cwd)
	setting.Initialize(cwd)
	if m.runtime.HookEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
	}
	m.runtime.ApplySettings(setting.DefaultSetup)
}

func (m *model) applyRuntimeHookOutcome(outcome hook.HookOutcome) {
	if outcome.InitialUserMessage != "" && m.initialPrompt == "" && len(m.conv.Messages) == 0 {
		m.initialPrompt = outcome.InitialUserMessage
	}
	if len(outcome.WatchPaths) == 0 {
		return
	}
	if m.fileWatcher == nil {
		queue := m.systemInput.AsyncHookQueue
		m.fileWatcher = trigger.NewFileWatcher(m.runtime.HookEngine, func(outcome hook.HookOutcome) {
			if queue != nil && outcome.InitialUserMessage != "" {
				queue.Push(trigger.AsyncHookRewake{
					Notice:  "File watcher hook triggered",
					Context: []string{outcome.InitialUserMessage},
				})
			}
		})
	}
	m.fileWatcher.SetPaths(outcome.WatchPaths)
}

// --- Output Runtime bridge (agent outbox → model mutation) ---

const autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

const minMessagesForCompaction = 3

type outputRuntime struct {
	*model
}

func (m *model) updateOutput(msg tea.Msg) (tea.Cmd, bool) {
	return conv.Update(outputRuntime{m}, &m.agentOutput, msg)
}

var _ conv.Runtime = outputRuntime{}

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
	m.runtime.InputTokens = in
	m.runtime.OutputTokens = out
}
func (m *model) ClearWarningSuppressed() { m.conv.Compact.WarningSuppressed = false }
func (m *model) ClearThinkingOverride()  { m.runtime.ThinkingOverride = llm.ThinkingOff }
func (rt outputRuntime) ContinueOutbox() tea.Cmd {
	return rt.model.continueOutbox()
}

func (m *model) PopToolSideEffect(toolCallID string) any {
	return tool.PopSideEffect(toolCallID)
}
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
	for _, drain := range []func() tea.Cmd{
		m.drainInputQueueToAgent,
		m.drainCronQueueToAgent,
		m.drainAsyncHookQueueToAgent,
		m.drainTaskNotificationsToAgent,
	} {
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
func (m *model) HandleTokenLimitResult(msg conv.TokenLimitResultMsg) tea.Cmd {
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
	m.userInput.Approval.Show(&perm.PermissionRequest{
		ToolName:    req.ToolName,
		Description: req.Description,
	}, m.width, m.height)
	return nil
}

type permissionDecision struct {
	Approved bool
	AllowAll bool
	Request  *perm.PermissionRequest
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

	resp := conv.PermBridgeResponse{
		Allow:  decision.Approved,
		Reason: "user decision",
	}

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

// --- Side effects ---

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

// --- Continuation injection helpers ---

func (m *model) drainInputQueueToAgent() tea.Cmd {
	if m.userInput.Queue.Len() == 0 {
		return nil
	}
	item, ok := m.userInput.Queue.Dequeue()
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
	items := notify.PopReadyNotifications(m.agentInput.Notifications, true)
	if len(items) == 0 {
		return nil
	}
	return m.injectTaskNotificationContinuation(notify.MergeNotifications(items))
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
	return conv.DrainAgentOutbox(m.agentSess.agent.Outbox())
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

// --- Compact helpers ---

func (m *model) getEffectiveInputLimit() int {
	return conv.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return conv.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens)
}

func (m *model) getContextUsagePercent() float64 {
	return conv.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) shouldAutoCompact() bool {
	return conv.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) triggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = conv.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.agentOutput.Spinner.Tick, compactCmd(m.buildCompactRequest("", "auto")))
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
	if m.fileCache != nil {
		restoredFiles, _ = m.fileCache.RestoreRecent()
		if len(restoredFiles) > 0 {
			restoredContext = filecache.FormatRestoredFiles(restoredFiles)
		}
	}

	if m.runtime.SessionStore != nil && m.runtime.SessionID != "" {
		_ = m.runtime.SessionStore.SaveSessionMemory(m.runtime.SessionID, msg.Summary)
	}
	m.runtime.SessionSummary = msg.Summary

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.ExecuteAsync(hook.PostCompact, hook.HookInput{
			Trigger: msg.Trigger,
		})
	}

	scrollPart := tea.Sequence(append(scrollbackCmds, tea.Println(boundary), tea.ClearScreen)...)
	cmds := []tea.Cmd{scrollPart}
	if shouldContinue {
		m.conv.Compact.ClearResult()
		if restoredContext != "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleUser,
				Content: restoredContext,
			})
		}
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: autoCompactResumePrompt,
		})
		cmds = append(cmds, m.sendToAgent(autoCompactResumePrompt, nil))
	} else if restoredContext != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: restoredContext,
		})
		m.conv.AddNotice(fmt.Sprintf("Restored %d recently accessed file(s) for context.", len(restoredFiles)))
		cmds = append(cmds, m.commitMessages()...)
	}
	return tea.Batch(cmds...)
}

func (m *model) resetAfterCompact() {
	m.conv.Clear()
	m.runtime.ResetTokens()
}

func (m *model) handleTokenLimitResult(msg conv.TokenLimitResultMsg) tea.Cmd {
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

// --- Session lifecycle ---

func (m *model) saveSession() error {
	if err := m.runtime.EnsureSessionStore(m.cwd); err != nil {
		return err
	}

	if len(m.conv.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(m.conv.Messages)

	providerName := ""
	modelID := ""
	if m.runtime.CurrentModel != nil {
		providerName = string(m.runtime.CurrentModel.Provider)
		modelID = m.runtime.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         m.runtime.SessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    m.runtime.SessionSummary,
			Mode:       m.runtime.SessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := m.runtime.SessionStore.Save(sess); err != nil {
		return err
	}

	m.runtime.SessionID = sess.Metadata.ID
	m.initTaskStorage()

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetTranscriptPath(m.runtime.SessionStore.SessionPath(sess.Metadata.ID))
	}

	m.reconfigureAgentTool()

	return nil
}

func (m *model) loadSession(id string) error {
	if err := m.runtime.EnsureSessionStore(m.cwd); err != nil {
		return err
	}

	sess, err := m.runtime.SessionStore.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	m.runtime.InputTokens = 0
	m.runtime.OutputTokens = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	m.runtime.SessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		m.runtime.SessionSummary = sess.Metadata.Summary
	} else if m.runtime.SessionStore != nil {
		if mem, err := m.runtime.SessionStore.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			m.runtime.SessionSummary = mem
		}
	}

	m.initTaskStorage()

	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

func (m *model) initTaskStorage() {
	if tracker.DefaultStore.GetStorageDir() != "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Logger().Warn("failed to get home directory for task storage", zap.Error(err))
		return
	}

	taskListID := os.Getenv("GEN_TASK_LIST_ID")
	if taskListID != "" {
		dir := filepath.Join(homeDir, ".gen", "tasks", taskListID)
		tracker.DefaultStore.SetStorageDir(dir)
		_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
		return
	}

	if m.runtime.SessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", m.runtime.SessionID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}

// --- Prompt suggestion (ghost text) ---

type promptSuggestionMsg struct {
	text string
	err  error
}

type promptSuggestionState struct {
	text   string
	cancel context.CancelFunc
}

func (s *promptSuggestionState) Clear() {
	s.text = ""
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

const suggestionSystemPrompt = `You predict what the user will type next in a coding assistant CLI.
Reply with ONLY the predicted text (2-12 words). No quotes, no explanation.
If unsure, reply with nothing.`

const suggestionUserPrompt = `[PREDICTION MODE] Based on this conversation, predict what the user will type next.
Stay silent if the next step isn't obvious. Match the user's language and style.`

const maxSuggestionMessages = 20

func (m *model) startPromptSuggestion() tea.Cmd {
	req, ok := m.buildPromptSuggestionRequest()
	if !ok {
		return nil
	}

	m.promptSuggestion.Clear()

	ctx, cancel := context.WithCancel(context.Background())
	m.promptSuggestion.cancel = cancel
	req.Ctx = ctx

	return suggestPromptCmd(req)
}

func (m *model) handlePromptSuggestion(msg promptSuggestionMsg) {
	if msg.err != nil {
		return
	}
	if m.userInput.Textarea.Value() != "" {
		return
	}
	if m.conv.Stream.Active {
		return
	}
	if text := suggest.FilterSuggestion(msg.text); text != "" {
		m.promptSuggestion.text = text
	}
}

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *llm.Client
	Messages     []core.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type compactRequest struct {
	Ctx            context.Context
	Client         *llm.Client
	Messages       []core.Message
	SessionSummary string
	Focus          string
	HookEngine     *hook.Engine
	Trigger        string
}

func suggestPromptCmd(req promptSuggestionRequest) tea.Cmd {
	if req.Client == nil {
		return nil
	}
	return func() tea.Msg {
		resp, err := req.Client.Complete(req.Ctx, req.SystemPrompt, req.Messages, req.MaxTokens)
		if err != nil {
			return promptSuggestionMsg{err: err}
		}
		return promptSuggestionMsg{text: resp.Content}
	}
}

func compactCmd(req compactRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := req.Ctx
		focus := req.Focus
		if req.HookEngine != nil {
			outcome := req.HookEngine.Execute(ctx, hook.PreCompact, hook.HookInput{
				Trigger:            req.Trigger,
				CustomInstructions: req.Focus,
			})
			if outcome.AdditionalContext != "" {
				if focus != "" {
					focus += "\n" + outcome.AdditionalContext
				} else {
					focus = outcome.AdditionalContext
				}
			}
		}
		summary, count, err := conv.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return conv.CompactResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

func (m *model) buildPromptSuggestionRequest() (promptSuggestionRequest, bool) {
	if m.runtime.LLMProvider == nil {
		return promptSuggestionRequest{}, false
	}

	assistantCount := 0
	for _, msg := range m.conv.Messages {
		if msg.Role == core.RoleAssistant {
			assistantCount++
		}
	}
	if assistantCount < 2 {
		return promptSuggestionRequest{}, false
	}

	startIdx := 0
	if len(m.conv.Messages) > maxSuggestionMessages {
		startIdx = len(m.conv.Messages) - maxSuggestionMessages
	}
	msgs := m.conv.ConvertToProviderFrom(startIdx)
	msgs = append(msgs, core.Message{
		Role:    core.RoleUser,
		Content: suggestionUserPrompt,
	})

	return promptSuggestionRequest{
		Client:       m.buildLoopClient(),
		Messages:     msgs,
		SystemPrompt: suggestionSystemPrompt,
		UserPrompt:   suggestionUserPrompt,
		MaxTokens:    60,
	}, true
}

func (m *model) buildCompactRequest(focus, trigger string) compactRequest {
	return compactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLoopClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.runtime.SessionSummary,
		Focus:          focus,
		HookEngine:     m.runtime.HookEngine,
		Trigger:        trigger,
	}
}

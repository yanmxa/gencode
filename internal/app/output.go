// output.Runtime implementation: mutation primitives that output event
// handlers call via the Runtime interface.  Also contains compact, session,
// and permission-bridge logic used by the output path.
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

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/kit"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/log"
)

const autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

const minMessagesForCompaction = 3

// --- Dispatcher ---

type outputRuntime struct {
	*model
}

func (m *model) updateOutput(msg tea.Msg) (tea.Cmd, bool) {
	return appoutput.Update(outputRuntime{m}, &m.agentOutput, msg)
}

// Compile-time checks: the adapter satisfies output.Runtime.
var _ appoutput.Runtime = outputRuntime{}

// --- output.Runtime: ConversationMutator ---

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
	return rt.model.outputContinueOutbox()
}

func (m *model) PopToolSideEffect(toolCallID string) any {
	return tool.PopSideEffect(toolCallID)
}
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
	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.ExecuteAsync(hook.StopFailure, hook.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			Error:                err.Error(),
			StopHookActive:       m.runtime.HookEngine.StopHookActive(),
		})
	}
}

func (m *model) StopAgentSession() {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
}

// --- output.Runtime: CompactHandler ---

func (m *model) HandleCompactResult(msg appoutput.CompactResultMsg) tea.Cmd {
	return m.handleCompactResult(msg)
}
func (m *model) HandleTokenLimitResult(msg appoutput.TokenLimitResultMsg) tea.Cmd {
	return m.handleTokenLimitResult(msg)
}

// --- output.Runtime: PermBridgeHandler ---

func (m *model) StorePendingPermRequest(req *appoutput.PermBridgeRequest) {
	if m.agentSess != nil {
		m.agentSess.pendingPermRequest = req
	}
}

func (m *model) ShowPermissionPrompt(req *appoutput.PermBridgeRequest) tea.Cmd {
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

	resp := appoutput.PermBridgeResponse{
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

	return appoutput.PollPermBridge(m.agentSess.permBridge)
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
	if m.runtime.HookEngine == nil {
		return
	}
	eventType := hook.PostToolUse
	if tr.IsError {
		eventType = hook.PostToolUseFailure
	}
	toolResponse := any(tr.Content)
	if sideEffect != nil {
		toolResponse = sideEffect
	}
	input := hook.HookInput{
		ToolName:     tr.ToolName,
		ToolUseID:    tr.ToolCallID,
		ToolResponse: toolResponse,
	}
	if tr.IsError {
		input.Error = tr.Content
	}
	m.runtime.HookEngine.ExecuteAsync(eventType, input)
}

func (m *model) fireIdleHooks() bool {
	if m.runtime.HookEngine == nil {
		return false
	}

	blocked := false
	if m.runtime.HookEngine.HasHooks(hook.Stop) {
		outcome := m.runtime.HookEngine.Execute(context.Background(), hook.Stop, hook.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			StopHookActive:       m.runtime.HookEngine.StopHookActive(),
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

	m.runtime.HookEngine.ExecuteAsync(hook.Notification, hook.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
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

func (m *model) outputContinueOutbox() tea.Cmd {
	return m.continueOutbox()
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
	if err := m.ensureSessionStore(); err == nil && m.runtime.SessionID != "" {
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
	return appoutput.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return appoutput.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens)
}

func (m *model) getContextUsagePercent() float64 {
	return appoutput.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) shouldAutoCompact() bool {
	return appoutput.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) triggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = appoutput.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.agentOutput.Spinner.Tick, compactCmd(m.buildCompactRequest("", "auto")))
	return tea.Batch(commitCmds...)
}

func (m *model) handleCompactResult(msg appoutput.CompactResultMsg) tea.Cmd {
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
	m.runtime.InputTokens = 0
	m.runtime.OutputTokens = 0
}

func (m *model) handleTokenLimitResult(msg appoutput.TokenLimitResultMsg) tea.Cmd {
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

func (m *model) ensureSessionStore() error {
	if m.runtime.SessionStore == nil {
		store, err := session.NewStore(m.cwd)
		if err != nil {
			return err
		}
		m.runtime.SessionStore = store
	}
	return nil
}

func (m *model) saveSession() error {
	if err := m.ensureSessionStore(); err != nil {
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
			Mode:       m.currentSessionMode(),
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
	if err := m.ensureSessionStore(); err != nil {
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

func (m *model) currentSessionMode() string {
	if m.runtime.PlanEnabled {
		return "plan"
	}
	switch m.runtime.OperationMode {
	case setting.ModeAutoAccept:
		return "auto-accept"
	default:
		return "normal"
	}
}

// --- Request types and tea.Cmd builders for compaction and prompt suggestion ---

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *llm.Client
	Messages     []core.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type tokenLimitFetchRequest struct {
	Ctx          context.Context
	LLM          llm.Provider
	Store        *llm.Store
	CurrentModel *llm.CurrentModelInfo
	Cwd          string
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

func fetchTokenLimitsCmd(req tokenLimitFetchRequest) tea.Cmd {
	deps := autoFetchTokenLimitsDeps{
		LLM:          req.LLM,
		Store:        req.Store,
		CurrentModel: req.CurrentModel,
		Cwd:          req.Cwd,
	}
	ctx := req.Ctx
	return func() tea.Msg {
		result, err := autoFetchTokenLimits(ctx, deps)
		return appoutput.TokenLimitResultMsg{Result: result, Error: err}
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
		summary, count, err := appoutput.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return appoutput.CompactResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
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

func (m *model) buildTokenLimitFetchRequest() tokenLimitFetchRequest {
	return tokenLimitFetchRequest{
		Ctx:          context.Background(),
		LLM:          m.runtime.LLMProvider,
		Store:        m.runtime.ProviderStore,
		CurrentModel: m.runtime.CurrentModel,
		Cwd:          m.cwd,
	}
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

// Root model: struct definition, construction, message pipeline, session
// persistence, conv.Runtime event handlers, deps builders, and internal helpers.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

const defaultWidth = 80

// ============================================================
// Model struct
// ============================================================

type model struct {
	// ── Sub-models (one per event source / concern) ─────────────
	userInput   input.Model    // Source 1: user keyboard input
	agentInput  notify.Model   // Source 2: background agent completion
	systemInput trigger.Model  // Source 3: system events (cron/hooks/watcher)
	conv        conv.Model     // Agent Outbox: conversation + output rendering
	env Env // Shared app state: provider, session, permission, plan, config

	// ── Agent session ───────────────────────────────────────────
	agentSess *agentSession

	// ── Infrastructure ──────────────────────────────────────────
	cwd           string
	isGit         bool
	width         int
	height        int
	ready         bool
	initialPrompt string
}

var (
	_ conv.Runtime          = (*model)(nil)
	_ input.SubmitRuntime   = (*model)(nil)
	_ input.ApprovalRuntime = (*model)(nil)
)

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
		m.conv.Spinner.Tick,
		m.userInput.MCP.Selector.AutoConnect(),
		trigger.TriggerCronTickNow(),
		trigger.StartCronTicker(),
		trigger.StartAsyncHookTicker(),
		notify.StartTicker(),
	}
	if m.initialPrompt != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return initialPromptMsg(prompt) })
	}
	return tea.Batch(cmds...)
}

// ============================================================
// Model construction
// ============================================================

func newModel(opts setting.RunOptions) (*model, error) {
	base := newBaseModel()
	m := &base
	notify.InstallCompletionObserver(m.agentInput.Notifications, hook.DefaultEngine)
	m.configureAsyncHookCallback()
	m.ensureMemoryContextLoaded()
	m.ReconfigureAgentTool()
	m.InitTaskStorage()
	if err := m.applyRunOptions(opts); err != nil {
		return nil, err
	}
	return m, nil
}

func newBaseModel() model {
	return model{
		userInput: input.New(appCwd, defaultWidth, commandSuggestionMatcher(), input.SelectorDeps{
			AgentRegistry:  &agentRegistryAdapter{subagent.DefaultRegistry},
			SkillRegistry:  skill.DefaultRegistry,
			MCPRegistry:    mcp.DefaultRegistry,
			PluginRegistry: plugin.DefaultRegistry,
			LoadDisabled:   setting.GetDisabledToolsAt,
			UpdateDisabled: setting.UpdateDisabledToolsAt,
		}),
		conv:        conv.NewModel(defaultWidth),
		agentInput:  notify.New(),
		systemInput: trigger.New(),
		env: newEnv(),

		cwd:   appCwd,
		isGit: setting.IsGitRepo(appCwd),
	}
}

func (m *model) applyRunOptions(opts setting.RunOptions) error {
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := plugin.DefaultRegistry.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
		}
		if err := m.ReloadPluginBackedState(); err != nil {
			return err
		}
	}

	if opts.Prompt != "" && !opts.PlanMode {
		m.initialPrompt = opts.Prompt
	}

	if opts.PlanMode {
		if err := m.enablePlanMode(opts.Prompt); err != nil {
			return err
		}
	}

	if opts.Continue {
		if err := m.applyContinueOption(); err != nil {
			return err
		}
	}

	if opts.Resume {
		if err := m.applyResumeOption(opts.ResumeID); err != nil {
			return err
		}
	}

	return nil
}

func (m *model) ReloadPluginBackedState() error {
	skill.Initialize(m.cwd)
	command.SetDynamicInfoProviders(skillCommandInfos)
	command.Initialize(m.cwd)
	subagent.Initialize(m.cwd, pluginAgentPaths)
	mcp.Initialize(m.cwd, pluginMCPServers)

	setting.Initialize(m.cwd)
	if hook.DefaultEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
	}
	syncSettingsToHookEngine()
	m.ReconfigureAgentTool()

	return nil
}

func (m *model) enablePlanMode(prompt string) error {
	m.env.PlanEnabled = true
	m.env.PlanTask = prompt
	m.env.OperationMode = setting.ModePlan

	planStore, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.env.PlanStore = planStore
	return nil
}

func (m *model) applyContinueOption() error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	session.DefaultSetup.Store = sessionStore

	sess, err := sessionStore.GetLatest()
	if err != nil {
		return fmt.Errorf("no previous session to continue: %w", err)
	}

	m.restoreSessionData(sess)
	return nil
}

func (m *model) applyResumeOption(resumeID string) error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	session.DefaultSetup.Store = sessionStore

	if resumeID != "" {
		sess, err := sessionStore.Load(resumeID)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", resumeID, err)
		}
		m.restoreSessionData(sess)
		return nil
	}

	m.userInput.Session.PendingSelector = true
	return nil
}

func (m *model) BuildCompactRequest(focus, trigger string) conv.CompactRequest {
	return conv.CompactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLLMClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: session.DefaultSetup.Summary,
		Focus:          focus,
		HookEngine:     hook.DefaultEngine,
		Trigger:        trigger,
	}
}

func (m *model) ensureMemoryContextLoaded() {
	if m.env.CachedUserInstructions != "" || m.env.CachedProjectInstructions != "" {
		return
	}
	m.env.RefreshMemoryContext(m.cwd, "session_start")
}

// ============================================================
// Message commit pipeline
// ============================================================

func (m *model) CommitMessages() []tea.Cmd {
	return m.renderAndCommit(true)
}

func (m *model) commitAllMessages() []tea.Cmd {
	return m.renderAndCommit(false)
}

func (m *model) renderAndCommit(checkReady bool) []tea.Cmd {
	var printCmds []tea.Cmd
	lastIdx := len(m.conv.Messages) - 1

	for i := m.conv.CommittedCount; i < len(m.conv.Messages); i++ {
		msg := m.conv.Messages[i]

		if checkReady {
			if i == lastIdx && msg.Role == core.RoleAssistant && m.conv.Stream.Active {
				break
			}
			if msg.Role == core.RoleAssistant && len(msg.ToolCalls) > 0 && !m.conv.HasAllToolResults(i) {
				break
			}
		}

		if rendered := conv.RenderSingleMessage(m.messageRenderParams(), i); rendered != "" {
			printCmds = append(printCmds, tea.Println(rendered))
		}
		m.conv.CommittedCount = i + 1
	}

	// Wrap in tea.Sequence to preserve message ordering.
	if len(printCmds) > 1 {
		return []tea.Cmd{tea.Sequence(printCmds...)}
	}
	return printCmds
}

// ============================================================
// Session persistence
// ============================================================

func (m *model) InitTaskStorage() {
	initTaskStorage(session.DefaultSetup.SessionID)
}

func (m *model) PersistSession() error {
	if err := session.EnsureStore(m.cwd); err != nil {
		return err
	}
	if len(m.conv.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(m.conv.Messages)

	var providerName, modelID string
	if m.env.CurrentModel != nil {
		providerName = string(m.env.CurrentModel.Provider)
		modelID = m.env.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         session.DefaultSetup.SessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    session.DefaultSetup.Summary,
			Mode:       m.env.SessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := session.DefaultSetup.Store.Save(sess); err != nil {
		return err
	}

	session.DefaultSetup.SessionID = sess.Metadata.ID
	initTaskStorage(session.DefaultSetup.SessionID)

	if hook.DefaultEngine != nil {
		hook.DefaultEngine.SetTranscriptPath(session.DefaultSetup.Store.SessionPath(sess.Metadata.ID))
	}
	m.ReconfigureAgentTool()

	return nil
}

func (m *model) loadSessionByID(id string) error {
	if err := session.EnsureStore(m.cwd); err != nil {
		return err
	}

	sess, err := session.DefaultSetup.Store.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	m.env.InputTokens = 0
	m.env.OutputTokens = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	session.DefaultSetup.SessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		session.DefaultSetup.Summary = sess.Metadata.Summary
	} else if session.DefaultSetup.Store != nil {
		if mem, err := session.DefaultSetup.Store.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			session.DefaultSetup.Summary = mem
		}
	}

	initTaskStorage(session.DefaultSetup.SessionID)

	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

func initTaskStorage(sessionID string) {
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

	if sessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", sessionID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}

// ============================================================
// conv.Runtime — agent outbox event handlers
// ============================================================

func (m *model) SetTokenCounts(in, out int) {
	m.env.InputTokens = in
	m.env.OutputTokens = out
}

func (m *model) HasRunningTasks() bool { return tracker.DefaultStore.HasInProgress() }

func (m *model) ProcessToolResult(tr core.ToolResult) *core.ToolResult {
	sideEffect := tool.PopSideEffect(tr.ToolCallID)
	if sideEffect != nil {
		m.applyToolSideEffects(tr.ToolName, sideEffect)
	}
	m.env.FirePostToolHook(tr, sideEffect)

	result := &core.ToolResult{
		ToolCallID: tr.ToolCallID,
		ToolName:   tr.ToolName,
		Content:    tr.Content,
		IsError:    tr.IsError,
	}
	m.persistOverflow(result)
	return result
}

func (m *model) ProcessTurnEnd(result core.Result) tea.Cmd {
	m.env.ClearThinkingOverride()
	commitCmds := m.CommitMessages()

	if blocked, sendCmd := m.fireIdleHooks(); blocked {
		cmds := append(commitCmds, m.ContinueOutbox())
		if sendCmd != nil {
			cmds = append(cmds, sendCmd)
		}
		return tea.Batch(cmds...)
	}

	if err := m.PersistSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}

	if kit.ShouldAutoCompact(m.env.LLMProvider, len(m.conv.Messages), m.env.InputTokens, llm.DefaultSetup.Store, m.env.CurrentModel) {
		m.conv.Compact.AutoContinue = true
		return tea.Batch(append(commitCmds, m.triggerAutoCompact())...)
	}

	if cmd := input.StartPromptSuggestion(m.promptSuggestionDeps()); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	if cmd := m.drainTurnQueues(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	if result.StopReason != "" && result.StopReason != core.StopEndTurn {
		m.conv.AddNotice(fmt.Sprintf("Agent stopped: %s", result.StopReason))
		if result.StopDetail != "" {
			m.conv.AddNotice(result.StopDetail)
		}
	}

	return tea.Batch(append(commitCmds, m.ContinueOutbox())...)
}

func (m *model) ProcessAgentStop(err error) tea.Cmd {
	if err != nil {
		m.conv.AddNotice(fmt.Sprintf("Agent error: %v", err))
		m.env.FireStopFailureHook(core.LastAssistantChatContent(m.conv.Messages), err)
	}
	commitCmds := m.CommitMessages()
	m.StopAgentSession()
	return tea.Batch(commitCmds...)
}

const autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

func (m *model) HandleCompactResult(msg conv.CompactResultMsg) tea.Cmd {
	shouldContinue := m.conv.Compact.AutoContinue
	if msg.Error != nil {
		m.conv.Compact.Complete(fmt.Sprintf("Compaction could not be completed: %v", msg.Error), true)
		return tea.Batch(m.CommitMessages()...)
	}
	m.conv.Compact.Complete(fmt.Sprintf("Condensed %d earlier messages.", msg.OriginalCount), false)
	scrollbackCmds := m.commitAllMessages()
	boundaryStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	boundary := boundaryStyle.Render(fmt.Sprintf("✻ Conversation compacted — %d messages summarized (scroll up for history)", msg.OriginalCount))

	m.conv.Clear()
	m.env.ResetTokens()

	var restoredFiles []filecache.RestoredFile
	var restoredContext string
	if m.env.FileCache != nil {
		restoredFiles, _ = m.env.FileCache.RestoreRecent()
		if len(restoredFiles) > 0 {
			restoredContext = filecache.FormatRestoredFiles(restoredFiles)
		}
	}
	if session.DefaultSetup.Store != nil && session.DefaultSetup.SessionID != "" {
		_ = session.DefaultSetup.Store.SaveSessionMemory(session.DefaultSetup.SessionID, msg.Summary)
	}
	session.DefaultSetup.Summary = msg.Summary
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.PostCompact, hook.HookInput{Trigger: msg.Trigger})
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
		cmds = append(cmds, m.CommitMessages()...)
	}
	return tea.Batch(cmds...)
}

func (m *model) HandleTokenLimitResult(msg kit.TokenLimitResultMsg) tea.Cmd {
	m.userInput.Provider.FetchingLimits = false
	var content string
	if msg.Error != nil {
		content = "Error: " + msg.Error.Error()
	} else {
		content = msg.Result
	}
	m.conv.AddNotice(content)
	return tea.Batch(m.CommitMessages()...)
}

// ============================================================
// Internal: tool side effects and context changes
// ============================================================

func (m *model) applyToolSideEffects(toolName string, sideEffect any) {
	resp, ok := sideEffect.(map[string]any)
	if !ok {
		return
	}
	m.syncBackgroundTaskTrackerFromAgent(toolName, resp)
	switch toolName {
	case "Bash":
		if newCwd := kit.MapString(resp, "cwd"); newCwd != "" {
			m.changeCwd(newCwd)
		}
	case tool.ToolEnterWorktree:
		if worktreePath := kit.MapString(resp, "worktreePath"); worktreePath != "" {
			m.changeCwd(worktreePath)
		}
	case tool.ToolExitWorktree:
		if restoredPath := kit.MapString(resp, "restoredPath"); restoredPath != "" {
			m.changeCwd(restoredPath)
		}
	case "Write", "Edit":
		if filePath := kit.MapString(resp, "filePath"); filePath != "" {
			m.fireFileChanged(filePath, toolName)
			if m.env.FileCache != nil {
				m.env.FileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := kit.MapString(fileData, "filePath"); filePath != "" && m.env.FileCache != nil {
				m.env.FileCache.Touch(filePath)
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
		TaskID:      kit.MapString(bg, "taskId"),
		AgentName:   kit.MapString(bg, "agentName"),
		AgentType:   kit.MapString(bg, "agentType"),
		Description: kit.MapString(bg, "description"),
		ResumeID:    kit.MapString(bg, "resumeId"),
	}
	if launch.TaskID == "" {
		return
	}
	childID := notify.EnsureBackgroundWorkerTracker(launch, "", "")
	if childID != "" {
		notify.RecordBackgroundTaskLaunch(launch, "", "", 0)
	}
}

func (m *model) persistOverflow(result *core.ToolResult) {
	const overflowThreshold = 100_000
	const previewSize = 10_000

	if len(result.Content) <= overflowThreshold {
		return
	}
	cutoff := previewSize
	if cutoff > len(result.Content) {
		cutoff = len(result.Content)
	}
	for cutoff > 0 && !utf8.RuneStart(result.Content[cutoff]) {
		cutoff--
	}
	preview := result.Content[:cutoff]
	persisted := false
	if err := session.EnsureStore(m.cwd); err == nil && session.DefaultSetup.SessionID != "" {
		if err := session.DefaultSetup.Store.PersistToolResult(session.DefaultSetup.SessionID, result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}
	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, session.DefaultSetup.SessionID, result.ToolCallID)
	} else {
		result.Content = fmt.Sprintf("%s\n\n[Output truncated from %d bytes — full content not persisted]", preview, len(result.Content))
	}
}

func (m *model) changeCwd(newCwd string) {
	if newCwd == "" || newCwd == m.cwd {
		return
	}
	oldCwd := m.cwd
	m.cwd = newCwd
	m.isGit = setting.IsGitRepo(newCwd)
	m.userInput.HandleCwdChange(newCwd)
	m.env.ClearCachedInstructions()
	m.env.RefreshMemoryContext(newCwd, "cwd_changed")
	m.ReloadProjectContext(newCwd)
	m.ReconfigureAgentTool()
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.SetCwd(newCwd)
		outcome := hook.DefaultEngine.Execute(context.Background(), hook.CwdChanged, hook.HookInput{OldCwd: oldCwd, NewCwd: newCwd})
		m.applyRuntimeHookOutcome(outcome)
	}
}

func (m *model) fireFileChanged(filePath, source string) {
	if hook.DefaultEngine == nil || filePath == "" {
		return
	}
	outcome := hook.DefaultEngine.Execute(context.Background(), hook.FileChanged, hook.HookInput{FilePath: filePath, Source: source, Event: "change"})
	m.applyRuntimeHookOutcome(outcome)
}

func (m *model) ReloadProjectContext(cwd string) {
	initExtensions(cwd)
	setting.Initialize(cwd)
	if hook.DefaultEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
	}
	syncSettingsToHookEngine()
}

func (m *model) applyRuntimeHookOutcome(outcome hook.HookOutcome) {
	if outcome.InitialUserMessage != "" && m.initialPrompt == "" && len(m.conv.Messages) == 0 {
		m.initialPrompt = outcome.InitialUserMessage
	}
	if len(outcome.WatchPaths) == 0 {
		return
	}
	if m.systemInput.FileWatcher == nil {
		m.systemInput.FileWatcher = trigger.NewFileWatcher(hook.DefaultEngine, func(outcome hook.HookOutcome) {
			if m.systemInput.AsyncHookQueue != nil && outcome.InitialUserMessage != "" {
				m.systemInput.AsyncHookQueue.Push(trigger.AsyncHookRewake{Notice: "File watcher hook triggered", Context: []string{outcome.InitialUserMessage}})
			}
		})
	}
	m.systemInput.FileWatcher.SetPaths(outcome.WatchPaths)
}

// ============================================================
// Internal: turn lifecycle and queue drain
// ============================================================

func (m *model) fireIdleHooks() (bool, tea.Cmd) {
	lastContent := core.LastAssistantChatContent(m.conv.Messages)
	blocked, reason := m.env.ExecuteIdleHooks(context.Background(), lastContent)
	if blocked {
		msg := "Stop hook blocked: " + reason
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: msg})
		return true, m.sendToAgent(msg, nil)
	}
	return false, nil
}

func (m *model) triggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = conv.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", kit.GetContextUsagePercent(m.env.InputTokens, llm.DefaultSetup.Store, m.env.CurrentModel)))
	commitCmds := m.CommitMessages()
	commitCmds = append(commitCmds, m.conv.Spinner.Tick, conv.CompactCmd(m.BuildCompactRequest("", "auto")))
	return tea.Batch(commitCmds...)
}

func (m *model) drainTurnQueues() tea.Cmd {
	if item, ok := m.userInput.Queue.Dequeue(); ok {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: item.Content, Images: item.Images})
		return m.sendToAgent(item.Content, item.Images)
	}

	if len(m.systemInput.CronQueue) > 0 {
		prompt := m.systemInput.CronQueue[0]
		m.systemInput.CronQueue = m.systemInput.CronQueue[1:]
		return m.injectCronPrompt(prompt)
	}

	if m.systemInput.AsyncHookQueue != nil {
		if item, ok := m.systemInput.AsyncHookQueue.Pop(); ok {
			return m.injectAsyncHookContinuation(item)
		}
	}

	if m.agentInput.Notifications != nil {
		if items := notify.PopReadyNotifications(m.agentInput.Notifications, true); len(items) > 0 {
			return m.injectTaskNotification(notify.MergeNotifications(items))
		}
	}

	return nil
}

func (m *model) injectTaskNotification(item notify.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if m.env.LLMProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no provider is connected."})
		}
		return tea.Batch(m.CommitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no task notification payload was available."})
		return tea.Batch(m.CommitMessages()...)
	}
	for _, ctx := range notify.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(notify.BuildContinuationPrompt(item), nil)
}

func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.env.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt)})
		return tea.Batch(m.CommitMessages()...)
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Scheduled task fired"})
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: prompt})
	return m.sendToAgent(prompt, nil)
}

func (m *model) injectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.CommitMessages()...)
	}
	if m.env.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Async hook requested a follow-up, but no provider is connected."})
		return tea.Batch(m.CommitMessages()...)
	}
	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

// ============================================================
// Deps builders and interface adapters
// ============================================================

func (m *model) promptSuggestionDeps() input.PromptSuggestionDeps {
	return input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv.ConversationModel,
		HasProvider:  m.env.LLMProvider != nil,
		BuildClient:  m.buildLLMClient,
	}
}

func (m *model) overlayDeps() input.OverlayDeps {
	return input.OverlayDeps{
		State:             &m.userInput,
		Conv:              &m.conv.ConversationModel,
		Cwd:               m.cwd,
		CommitMessages:    m.CommitMessages,
		CommitAllMessages: m.commitAllMessages,
		SwitchProvider: func(p llm.Provider) {
			m.env.SwitchProvider(p)
			m.ReconfigureAgentTool()
		},
		SetCurrentModel: func(info *llm.CurrentModelInfo) {
			m.env.CurrentModel = info
		},
		ClearCachedInstructions: m.env.ClearCachedInstructions,
		RefreshMemoryContext:    m.env.RefreshMemoryContext,
		FireFileChanged:         m.fireFileChanged,
		ReloadPluginState:       m.ReloadPluginBackedState,
		LoadSession:             m.loadSessionByID,
	}
}

func (m *model) notifyDeps() notify.Deps {
	return notify.Deps{
		StreamActive: m.conv.Stream.Active,
		Inject:       m.injectTaskNotification,
	}
}

func (m *model) triggerDeps() trigger.Deps {
	return trigger.Deps{
		StreamActive: m.conv.Stream.Active,
		InjectCron:   m.injectCronPrompt,
		InjectHook:   m.injectAsyncHookContinuation,
		AppendNotice: func(text string) {
			if text != "" {
				m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: text})
			}
		},
	}
}

func (m *model) StartExternalEditor(path string) tea.Cmd {
	return kit.StartExternalEditor(path, func(err error) tea.Msg {
		return input.MemoryEditorFinishedMsg{Err: err}
	})
}

func (m *model) SpinnerTickCmd() tea.Cmd { return m.conv.Spinner.Tick }
func (m *model) ResetCronQueue()         { m.systemInput.CronQueue = nil }

func (m *model) enterPlanModeForCommand(task string) error {
	m.env.OperationMode = setting.ModePlan
	m.env.PlanEnabled = true
	m.env.PlanTask = task
	m.env.SessionPermissions.AllowAllEdits = false
	m.env.SessionPermissions.AllowAllWrites = false
	m.env.SessionPermissions.AllowAllBash = false
	m.env.SessionPermissions.AllowAllSkills = false
	store, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.env.PlanStore = store
	return nil
}

func (m *model) forkSession() (string, error) {
	if session.DefaultSetup.SessionID == "" {
		return "", fmt.Errorf("no active session to fork")
	}
	forked, err := session.DefaultSetup.Store.Fork(session.DefaultSetup.SessionID)
	if err != nil {
		return "", err
	}
	originalID := forked.Metadata.ParentSessionID
	session.DefaultSetup.SessionID = forked.Metadata.ID
	session.DefaultSetup.Summary = ""
	tracker.DefaultStore.SetStorageDir("")
	return originalID, nil
}

func (m *model) FireSessionEnd(reason string) {
	m.env.FireSessionEnd(context.Background(), reason)
	if m.systemInput.FileWatcher != nil {
		m.systemInput.FileWatcher.Stop()
	}
}

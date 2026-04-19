// Root model: struct definition, construction, message pipeline, session
// persistence, conv.Runtime event handlers, deps builders, and internal helpers.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	"github.com/yanmxa/gencode/internal/tool"
)

const defaultWidth = 80

// ============================================================
// Model struct
// ============================================================

type model struct {
	// ── Sub-models (one per event source / concern) ─────────────
	userInput   input.Model   // Source 1: user keyboard input
	agentInput  notify.Model  // Source 2: background agent completion
	systemInput trigger.Model // Source 3: system events (cron/hooks/watcher)
	conv        conv.Model    // Agent Outbox: conversation + output rendering
	env         env           // Shared app state: provider, session, permission, plan, config
	services    services      // Domain service singletons, injected at construction
}

var (
	_ conv.Runtime          = (*model)(nil)
	_ input.SubmitRuntime   = (*model)(nil)
	_ input.ApprovalRuntime = (*model)(nil)
)

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
		m.userInput.MCP.Selector.AutoConnect(),
		trigger.TriggerCronTickNow(),
		trigger.StartCronTicker(),
		trigger.StartAsyncHookTicker(),
		notify.StartTicker(),
	}
	if m.env.InitialPrompt != "" {
		prompt := m.env.InitialPrompt
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
	m.agentInput.BGTracker = notify.NewBackgroundTracker(m.services.Tracker, m.services.Orchestration)
	var hookEngine *hook.Engine
	if m.services.Hook != nil {
		hookEngine = m.services.Hook.Engine()
	}
	notify.InstallCompletionObserver(m.agentInput.Notifications, hookEngine, m.agentInput.BGTracker)
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
	svc := newServices()
	return model{
		userInput: input.New(appCwd, defaultWidth, commandSuggestionMatcher(svc.Command), input.SelectorDeps{
			AgentRegistry:  &agentRegistryAdapter{svc.Subagent.Registry()},
			SkillRegistry:  svc.Skill.Registry(),
			MCPRegistry:    svc.MCP.Registry(),
			PluginRegistry: svc.Plugin.Registry(),
			Setting:        svc.Setting,
			LoadDisabled:   svc.Setting.GetDisabledToolsAt,
			UpdateDisabled: svc.Setting.UpdateDisabledToolsAt,
		}),
		conv:        conv.NewModel(defaultWidth),
		agentInput:  notify.New(),
		systemInput: trigger.New(),
		env:      newEnv(svc.LLM, appCwd, svc.Setting.IsGitRepo(appCwd)),
		services: svc,
	}
}

func (m *model) applyRunOptions(opts setting.RunOptions) error {
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := m.services.Plugin.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
		}
		if err := m.ReloadPluginBackedState(); err != nil {
			return err
		}
	}

	if opts.Prompt != "" && !opts.PlanMode {
		m.env.InitialPrompt = opts.Prompt
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
	skill.Initialize(skill.Options{CWD: m.env.CWD})
	command.Initialize(command.Options{
		CWD:              m.env.CWD,
		DynamicProviders: []func() []command.Info{skillCommandInfos},
		PluginCommandPaths: pluginCommandPaths,
	})
	subagent.Initialize(subagent.Options{CWD: m.env.CWD, PluginAgentPaths: pluginAgentPaths})
	mcp.Initialize(mcp.Options{CWD: m.env.CWD, PluginServers: pluginMCPServers})
	setting.Initialize(setting.Options{CWD: m.env.CWD})

	m.services.refreshAfterReload()

	if m.services.Hook != nil {
		plugin.MergePluginHooksIntoSettings(m.services.Setting.Snapshot())
	}
	m.syncSettingsToHookEngine()
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
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}

	sess, err := m.services.Session.LoadLatest()
	if err != nil {
		return fmt.Errorf("no previous session to continue: %w", err)
	}

	m.restoreSessionData(sess)
	return nil
}

func (m *model) applyResumeOption(resumeID string) error {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}

	if resumeID != "" {
		sess, err := m.services.Session.Load(resumeID)
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
	var hookEngine *hook.Engine
	if m.services.Hook != nil {
		hookEngine = m.services.Hook.Engine()
	}
	return conv.CompactRequest{
		Ctx:        context.Background(),
		Client:     m.buildLLMClient(),
		Messages:   m.conv.ConvertToProvider(),
		Focus:      focus,
		HookEngine: hookEngine,
		Trigger:    trigger,
	}
}

func (m *model) ensureMemoryContextLoaded() {
	if m.env.CachedUserInstructions != "" || m.env.CachedProjectInstructions != "" {
		return
	}
	m.refreshMemoryContext(m.env.CWD, "session_start")
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
	m.initTaskStorage(m.services.Session.ID())
}

func (m *model) PersistSession() error {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
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
			ID:         m.services.Session.ID(),
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.env.CWD,
			LastPrompt: session.ExtractLastUserText(entries),
			Mode:       m.env.SessionMode(),
		},
		Entries: entries,
		Tasks:   m.services.Tracker.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := m.services.Session.Save(sess); err != nil {
		return err
	}

	m.services.Session.SetID(sess.Metadata.ID)
	m.initTaskStorage(m.services.Session.ID())

	if m.services.Hook != nil {
		m.services.Hook.SetTranscriptPath(m.services.Session.GetStore().SessionPath(sess.Metadata.ID))
	}
	m.ReconfigureAgentTool()

	return nil
}

func (m *model) loadSessionByID(id string) error {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		return err
	}

	sess, err := m.services.Session.Load(id)
	if err != nil {
		return err
	}

	m.services.Tracker.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		m.services.Tracker.Reset()
	}
	m.services.Tool.ResetFetched()

	m.env.InputTokens = 0
	m.env.OutputTokens = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	m.services.Session.SetID(sess.Metadata.ID)

	m.initTaskStorage(m.services.Session.ID())

	if len(sess.Tasks) > 0 {
		m.services.Tracker.Import(sess.Tasks)
	}
}

func (m *model) initTaskStorage(sessionID string) {
	if m.services.Tracker.GetStorageDir() != "" {
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
		m.services.Tracker.SetStorageDir(dir)
		_ = m.services.Task.SetOutputDir(filepath.Join(dir, "outputs"))
		return
	}

	if sessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", sessionID)
	m.services.Tracker.SetStorageDir(dir)
	_ = m.services.Task.SetOutputDir(filepath.Join(dir, "outputs"))
}

// ============================================================
// conv.Runtime — agent outbox event handlers
// ============================================================

func (m *model) SetTokenCounts(in, out int) {
	m.env.InputTokens = in
	m.env.OutputTokens = out
}

func (m *model) HasRunningTasks() bool { return m.services.Tracker.HasInProgress() }

func (m *model) ProcessToolResult(tr core.ToolResult) *core.ToolResult {
	sideEffect := m.services.Tool.PopSideEffect(tr.ToolCallID)
	if sideEffect != nil {
		m.applyToolSideEffects(tr.ToolName, sideEffect)
	}
	m.firePostToolHook(tr, sideEffect)

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
	start := time.Now()
	queueLen := m.userInput.Queue.Len()
	log.QueueLog("ProcessTurnEnd: starting queueLen=%d", queueLen)
	commitCmds := m.CommitMessages()
	log.QueueLog("ProcessTurnEnd: CommitMessages done elapsed=%v", time.Since(start))

	// Drain queued messages FIRST — skip idle hooks when we have pending work,
	// since we're not truly idle and hooks like Stop/Notification would add latency.
	if cmd, found := m.drainTurnQueues(); found {
		log.QueueLog("ProcessTurnEnd: drained queued messages elapsed=%v", time.Since(start))
		if cmd != nil {
			commitCmds = append(commitCmds, cmd)
		}
		commitCmds = append(commitCmds, m.ContinueOutbox())
		return tea.Batch(commitCmds...)
	}

	if blocked, sendCmd := m.fireIdleHooks(); blocked {
		log.QueueLog("ProcessTurnEnd: idle hooks BLOCKED elapsed=%v", time.Since(start))
		cmds := append(commitCmds, m.ContinueOutbox())
		if sendCmd != nil {
			cmds = append(cmds, sendCmd)
		}
		return tea.Batch(cmds...)
	}
	log.QueueLog("ProcessTurnEnd: idle hooks done elapsed=%v", time.Since(start))

	if err := m.PersistSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}
	log.QueueLog("ProcessTurnEnd: persist done elapsed=%v", time.Since(start))

	if cmd := input.StartPromptSuggestion(m.promptSuggestionDeps()); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	if result.StopReason != "" && result.StopReason != core.StopEndTurn {
		m.conv.AddNotice(fmt.Sprintf("Agent stopped: %s", result.StopReason))
		if result.StopDetail != "" {
			m.conv.AddNotice(result.StopDetail)
		}
	}

	log.QueueLog("ProcessTurnEnd: returning elapsed=%v cmds=%d", time.Since(start), len(commitCmds)+1)
	return tea.Batch(append(commitCmds, m.ContinueOutbox())...)
}

func (m *model) ProcessAgentStop(err error) tea.Cmd {
	if err != nil {
		m.conv.AddNotice(fmt.Sprintf("Agent error: %v", err))
		m.fireStopFailureHook(core.LastAssistantChatContent(m.conv.Messages), err)
	}
	commitCmds := m.CommitMessages()
	m.StopAgentSession()
	return tea.Batch(commitCmds...)
}

func (m *model) HandleAgentCompact(info core.CompactInfo) tea.Cmd {
	scrollbackCmds := m.commitAllMessages()
	boundaryStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	boundary := boundaryStyle.Render(fmt.Sprintf("✻ Conversation compacted — %d messages summarized (scroll up for history)", info.OriginalCount))

	m.conv.Clear()
	m.env.ResetTokens()
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: core.FormatCompactSummary(info.Summary)})

	if m.services.Hook != nil {
		m.services.Hook.ExecuteAsync(hook.PostCompact, hook.HookInput{Trigger: "auto"})
	}

	scrollPart := tea.Sequence(append(scrollbackCmds, tea.Println(boundary), tea.ClearScreen)...)
	return tea.Batch(scrollPart, m.ContinueOutbox())
}

// HandleCompactResult handles manual /compact results.
// Stops the agent so the next user message restarts it with compacted messages.
func (m *model) HandleCompactResult(msg conv.CompactResultMsg) tea.Cmd {
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
	m.StopAgentSession()

	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: core.FormatCompactSummary(msg.Summary)})

	var restoredFiles []filecache.RestoredFile
	if m.env.FileCache != nil {
		restoredFiles, _ = m.env.FileCache.RestoreRecent()
		if len(restoredFiles) > 0 {
			restoredContext := filecache.FormatRestoredFiles(restoredFiles)
			m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: restoredContext})
			m.conv.AddNotice(fmt.Sprintf("Restored %d recently accessed file(s) for context.", len(restoredFiles)))
		}
	}
	if m.services.Hook != nil {
		m.services.Hook.ExecuteAsync(hook.PostCompact, hook.HookInput{Trigger: msg.Trigger})
	}

	scrollPart := tea.Sequence(append(scrollbackCmds, tea.Println(boundary), tea.ClearScreen)...)
	return tea.Batch(scrollPart, tea.Batch(m.CommitMessages()...))
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
	childID := m.agentInput.BGTracker.EnsureWorkerTracker(launch, "", "")
	if childID != "" {
		m.agentInput.BGTracker.RecordLaunch(launch, "", "", 0)
	}
}

func (m *model) persistOverflow(result *core.ToolResult) {
	const overflowThreshold = 100_000
	const previewSize = 10_000

	if len(result.Content) <= overflowThreshold {
		return
	}
	cutoff := min(previewSize, len(result.Content))
	for cutoff > 0 && !utf8.RuneStart(result.Content[cutoff]) {
		cutoff--
	}
	preview := result.Content[:cutoff]
	persisted := false
	if err := m.services.Session.EnsureStore(m.env.CWD); err == nil && m.services.Session.ID() != "" {
		if err := m.services.Session.GetStore().PersistToolResult(m.services.Session.ID(), result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}
	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.services.Session.ID(), result.ToolCallID)
	} else {
		result.Content = fmt.Sprintf("%s\n\n[Output truncated from %d bytes — full content not persisted]", preview, len(result.Content))
	}
}

func (m *model) changeCwd(newCwd string) {
	if newCwd == "" || newCwd == m.env.CWD {
		return
	}
	oldCwd := m.env.CWD
	m.env.CWD = newCwd
	m.env.IsGit = m.services.Setting.IsGitRepo(newCwd)
	m.userInput.HandleCwdChange(newCwd)
	m.env.ClearCachedInstructions()
	m.refreshMemoryContext(newCwd, "cwd_changed")
	m.ReloadProjectContext(newCwd)
	m.ReconfigureAgentTool()
	if m.services.Hook != nil {
		m.services.Hook.SetCwd(newCwd)
		outcome := m.services.Hook.Execute(context.Background(), hook.CwdChanged, hook.HookInput{OldCwd: oldCwd, NewCwd: newCwd})
		m.applyRuntimeHookOutcome(outcome)
	}
}

func (m *model) fireFileChanged(filePath, source string) {
	if m.services.Hook == nil || filePath == "" {
		return
	}
	outcome := m.services.Hook.Execute(context.Background(), hook.FileChanged, hook.HookInput{FilePath: filePath, Source: source, Event: "change"})
	m.applyRuntimeHookOutcome(outcome)
}

func (m *model) ReloadProjectContext(cwd string) {
	initExtensions(cwd)
	setting.Initialize(setting.Options{CWD: cwd})
	m.services.refreshAfterReload()
	if m.services.Hook != nil {
		plugin.MergePluginHooksIntoSettings(m.services.Setting.Snapshot())
	}
	m.syncSettingsToHookEngine()
}

func (m *model) applyRuntimeHookOutcome(outcome hook.HookOutcome) {
	if outcome.InitialUserMessage != "" && m.env.InitialPrompt == "" && len(m.conv.Messages) == 0 {
		m.env.InitialPrompt = outcome.InitialUserMessage
	}
	if len(outcome.WatchPaths) == 0 {
		return
	}
	if m.systemInput.FileWatcher == nil {
		m.systemInput.FileWatcher = trigger.NewFileWatcher(m.services.Hook.Engine(), func(outcome hook.HookOutcome) {
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
	blocked, reason := m.executeIdleHooks(context.Background(), lastContent)
	if blocked {
		msg := "Stop hook blocked: " + reason
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: msg})
		return true, m.sendToAgent(msg, nil)
	}
	return false, nil
}


func (m *model) drainTurnQueues() (tea.Cmd, bool) {
	// Drain ONE user message per call so each gets its own agent response.
	// The agent's inner loop also drains one inbox message at a time,
	// producing one TurnEvent per queued message.
	if item, ok := m.userInput.Queue.Dequeue(); ok {
		remaining := m.userInput.Queue.Len()
		log.QueueLog("drainTurnQueues: dequeued %q sentToInbox=%v remaining=%d", truncate(item.Content, 60), item.SentToInbox, remaining)
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: item.Content, Images: item.Images})
		if !item.SentToInbox {
			log.QueueLog("drainTurnQueues: sending to inbox (was not sent)")
			m.services.Agent.Send(item.Content, item.Images)
		}
		return nil, true
	}

	if len(m.systemInput.CronQueue) > 0 {
		prompt := m.systemInput.CronQueue[0]
		m.systemInput.CronQueue = m.systemInput.CronQueue[1:]
		return m.injectCronPrompt(prompt), true
	}

	if m.systemInput.AsyncHookQueue != nil {
		if item, ok := m.systemInput.AsyncHookQueue.Pop(); ok {
			return m.injectAsyncHookContinuation(item), true
		}
	}

	if m.agentInput.Notifications != nil {
		if items := notify.PopReadyNotifications(m.agentInput.Notifications, true); len(items) > 0 {
			return m.injectTaskNotification(notify.MergeNotifications(items)), true
		}
	}

	return nil, false
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
		Cwd:               m.env.CWD,
		CommitMessages:    m.CommitMessages,
		CommitAllMessages: m.commitAllMessages,
		SwitchProvider: func(p llm.Provider) {
			m.switchProvider(p)
			m.ReconfigureAgentTool()
		},
		SetCurrentModel: func(info *llm.CurrentModelInfo) {
			m.env.CurrentModel = info
		},
		ClearCachedInstructions: m.env.ClearCachedInstructions,
		RefreshMemoryContext:    m.refreshMemoryContext,
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
		Cron:         m.services.Cron,
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
	if m.services.Session.ID() == "" {
		return "", fmt.Errorf("no active session to fork")
	}
	forked, err := m.services.Session.Fork(m.services.Session.ID())
	if err != nil {
		return "", err
	}
	originalID := forked.Metadata.ParentSessionID
	m.services.Session.SetID(forked.Metadata.ID)
	m.services.Tracker.SetStorageDir("")
	return originalID, nil
}

func (m *model) FireSessionEnd(reason string) {
	if m.services.Hook != nil {
		m.services.Hook.Execute(context.Background(), hook.SessionEnd, hook.HookInput{
			Reason: reason,
		})
		m.services.Hook.ClearSessionHooks()
	}
	if m.systemInput.FileWatcher != nil {
		m.systemInput.FileWatcher.Stop()
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

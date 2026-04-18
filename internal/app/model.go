// Model struct, construction, agent session lifecycle, system prompt, LLM configuration,
// message pipeline, session persistence, and runtime interface implementations.
package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/notify"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
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
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

const defaultWidth = 80

// ============================================================
// Model struct
// ============================================================

type model struct {
	// ── Sub-models (one per event source) ────────────────────────
	userInput   input.Model            // Source 1: user keyboard input
	agentInput  notify.Model           // Source 2: background agent completion
	systemInput trigger.Model          // Source 3: system events (cron/hooks/watcher)
	conv        conv.ConversationModel // Agent Outbox: messages, modal, tool exec
	agentOutput conv.OutputModel       // Agent Outbox: spinner, markdown, progress
	runtime     appruntime.Model       // Shared: provider, session, permission, config

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

var _ conv.Runtime = (*model)(nil)
var _ input.Runtime = (*model)(nil)
var _ input.CommandRuntime = (*model)(nil)
var _ input.SubmitRuntime = (*model)(nil)
var _ input.ApprovalRuntime = (*model)(nil)
var _ notify.Runtime = (*model)(nil)
var _ trigger.Runtime = (*model)(nil)

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.agentOutput.Spinner.Tick, m.userInput.MCP.Selector.AutoConnect(), trigger.TriggerCronTickNow(), trigger.StartCronTicker(), trigger.StartAsyncHookTicker(), notify.StartTicker()}
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
	notify.InstallCompletionObserver(m.agentInput.Notifications)
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
	progressHub := conv.NewProgressHub(100)

	return model{
		userInput: input.New(appCwd, defaultWidth, commandSuggestionMatcher(), input.SelectorDeps{
			AgentRegistry:  &agentRegistryAdapter{subagent.DefaultRegistry},
			SkillRegistry:  skill.DefaultRegistry,
			MCPRegistry:    mcp.DefaultRegistry,
			PluginRegistry: plugin.DefaultRegistry,
			LoadDisabled:   setting.GetDisabledToolsAt,
			UpdateDisabled: setting.UpdateDisabledToolsAt,
		}),
		agentOutput: conv.New(defaultWidth, progressHub),
		conv:        conv.NewConversation(),
		agentInput:  notify.New(),
		systemInput: trigger.New(hook.DefaultEngine),
		runtime:     appruntime.New(appCwd),

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
		if err := m.reloadPluginBackedState(); err != nil {
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

func (m *model) reloadPluginBackedState() error {
	skill.Initialize(m.cwd)
	command.SetDynamicInfoProviders(skillCommandInfos)
	command.Initialize(m.cwd)
	subagent.Initialize(m.cwd, pluginAgentPaths)
	mcp.Initialize(m.cwd, pluginMCPServers)

	setting.Initialize(m.cwd)
	m.runtime.Settings = setting.DefaultSetup
	if m.runtime.HookEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
		m.runtime.HookEngine.SetSettings(setting.DefaultSetup)
	}
	m.ReconfigureAgentTool()

	return nil
}

func (m *model) enablePlanMode(prompt string) error {
	m.runtime.PlanEnabled = true
	m.runtime.PlanTask = prompt
	m.runtime.OperationMode = setting.ModePlan

	planStore, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.runtime.PlanStore = planStore
	return nil
}

func (m *model) applyContinueOption() error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.runtime.SessionStore = sessionStore

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
	m.runtime.SessionStore = sessionStore

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

// ============================================================
// Agent session lifecycle
// ============================================================

type agentSession struct {
	agent              core.Agent
	permBridge         *conv.PermissionBridge
	cancel             context.CancelFunc
	pendingPermRequest *conv.PermBridgeRequest
}

var errNoProvider = providerRequiredError("no LLM provider configured")

type providerRequiredError string

func (e providerRequiredError) Error() string { return string(e) }

func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.runtime.LLMProvider == nil {
		return nil, errNoProvider
	}

	client := llm.NewClient(m.runtime.LLMProvider, m.runtime.GetModelID(), kit.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens))
	client.SetThinking(m.runtime.EffectiveThinkingLevel())

	sys := m.buildSystemPrompt(nil, client)
	tools := m.buildAgentTools()

	permBridge := conv.NewPermissionBridge(func(name string, args map[string]any) conv.PermDecisionResult {
		settings := m.runtime.Settings
		if settings == nil {
			return conv.PermDecisionResult{Decision: perm.Permit}
		}
		decision := settings.HasPermissionToUseTool(name, args, m.runtime.SessionPermissions)
		switch decision.Behavior {
		case setting.Allow:
			return conv.PermDecisionResult{Decision: perm.Permit, Reason: decision.Reason}
		case setting.Deny:
			return conv.PermDecisionResult{Decision: perm.Reject, Reason: decision.Reason}
		default:
			return conv.PermDecisionResult{
				Decision:    perm.Prompt,
				Reason:      decision.Reason,
				ToolName:    name,
				Description: decision.Reason,
			}
		}
	})

	ag := core.NewAgent(core.Config{
		ID:     "main",
		LLM:    client,
		System: sys,
		Tools:  tool.WithPermission(tools, permBridge.PermissionFunc()),
		CWD:    m.cwd,
	})

	return &agentSession{agent: ag, permBridge: permBridge}, nil
}

func (m *model) startAgentLoop(sess *agentSession) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	sess.cancel = cancel

	go func() {
		_ = sess.agent.Run(ctx)
	}()

	return tea.Batch(
		conv.DrainAgentOutbox(sess.agent.Outbox()),
		conv.PollPermBridge(sess.permBridge),
	)
}

func (sess *agentSession) stop() {
	if sess == nil {
		return
	}
	if sess.cancel != nil {
		sess.cancel()
		sess.cancel = nil
	}
	if sess.permBridge != nil {
		sess.permBridge.Close()
	}
	if sess.agent != nil {
		select {
		case sess.agent.Inbox() <- core.Message{Signal: core.SigStop}:
		default:
		}
	}
}

func (m *model) ensureAgentSession() error {
	if m.agentSess != nil {
		return nil
	}
	sess, err := m.buildCoreAgent()
	if err != nil {
		return err
	}
	m.agentSess = sess

	if len(m.conv.Messages) > 0 {
		var coreMessages []core.Message
		for _, msg := range m.conv.ConvertToProvider() {
			coreMessages = append(coreMessages, msg)
		}
		sess.agent.SetMessages(coreMessages)
	}

	m.startAgentLoop(sess)
	return nil
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

// ============================================================
// Agent tool configuration
// ============================================================

func (m *model) ReconfigureAgentTool() {
	if m.runtime.LLMProvider == nil {
		return
	}
	m.ensureMemoryContextLoaded()

	executor := subagent.NewExecutor(m.runtime.LLMProvider, m.cwd, m.runtime.GetModelID(), m.runtime.HookEngine)
	if m.runtime.SessionStore != nil && m.runtime.SessionID != "" {
		executor.SetSessionStore(m.runtime.SessionStore, m.runtime.SessionID)
	}
	executor.SetContext(m.runtime.CachedUserInstructions, m.runtime.CachedProjectInstructions, m.isGit)
	if mcp.DefaultRegistry != nil {
		executor.SetMCP(mcp.DefaultRegistry.GetToolSchemas, mcp.DefaultRegistry)
	}

	adapter := subagent.NewExecutorAdapter(executor)
	if t, ok := tool.Get(tool.ToolAgent); ok {
		if agentTool, ok := t.(*toolagent.AgentTool); ok {
			agentTool.SetExecutor(adapter)
		}
	}
	if t, ok := tool.Get(tool.ToolContinueAgent); ok {
		if continueTool, ok := t.(*toolagent.ContinueAgentTool); ok {
			continueTool.SetExecutor(adapter)
		}
	}
	if t, ok := tool.Get(tool.ToolSendMessage); ok {
		if sendMessageTool, ok := t.(*toolagent.SendMessageTool); ok {
			sendMessageTool.SetExecutor(adapter)
		}
	}
}

// ============================================================
// System prompt and tool set
// ============================================================

func (m *model) buildSystemPrompt(extra []string, loopClient *llm.Client) core.System {
	var providerName, modelID string
	if loopClient != nil {
		modelID = loopClient.ModelID()
		providerName = loopClient.Name()
	}

	allExtra := append([]string{}, extra...)
	if coordinator := system.CoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.userInput.Skill.ActiveInvocation)
	}

	var sessionSummary string
	if m.runtime.SessionSummary != "" {
		sessionSummary = fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.runtime.SessionSummary)
	}

	var skills, agents string
	if skill.DefaultRegistry != nil {
		skills = skill.DefaultRegistry.GetSkillsSection()
	}
	if subagent.DefaultRegistry != nil {
		agents = subagent.DefaultRegistry.GetAgentsSection()
	}

	return system.Build(system.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.runtime.PlanEnabled,
		UserInstructions:    m.runtime.CachedUserInstructions,
		ProjectInstructions: m.runtime.CachedProjectInstructions,
		SessionSummary:      sessionSummary,
		Skills:              skills,
		Agents:              agents,
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               allExtra,
	})
}

func (m *model) buildAgentTools() core.Tools {
	var mcpGetter func() []core.ToolSchema
	if mcp.DefaultRegistry != nil {
		mcpGetter = mcp.DefaultRegistry.GetToolSchemas
	}
	schemas := (&tool.Set{
		Disabled: m.runtime.DisabledTools,
		PlanMode: m.runtime.PlanEnabled,
		MCP:      mcpGetter,
	}).Tools()

	tools := tool.AdaptToolRegistry(schemas, func() string { return m.cwd })
	if mcp.DefaultRegistry != nil {
		mcpCaller := mcp.NewCaller(mcp.DefaultRegistry)
		for _, t := range mcp.AsCoreTools(schemas, mcpCaller) {
			tools.Add(t)
		}
	}
	return tools
}

// ============================================================
// LLM client helpers
// ============================================================

func (m *model) buildLLMClient() *llm.Client {
	c := llm.NewClient(m.runtime.LLMProvider, m.runtime.GetModelID(), kit.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens))
	c.SetThinking(m.runtime.EffectiveThinkingLevel())
	return c
}

func (m *model) BuildCompactRequest(focus, trigger string) conv.CompactRequest {
	return conv.CompactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLLMClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.runtime.SessionSummary,
		Focus:          focus,
		HookEngine:     m.runtime.HookEngine,
		Trigger:        trigger,
	}
}

func (m *model) ensureMemoryContextLoaded() {
	if m.runtime.CachedUserInstructions != "" || m.runtime.CachedProjectInstructions != "" {
		return
	}
	m.runtime.RefreshMemoryContext(m.cwd, "session_start")
}


// ============================================================
// Message commit pipeline
// ============================================================

func (m *model) commitMessages() []tea.Cmd {
	return m.commitMessagesImpl(true)
}

func (m *model) commitAllMessages() []tea.Cmd {
	return m.commitMessagesImpl(false)
}

func (m *model) commitMessagesImpl(checkReady bool) []tea.Cmd {
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

func (m *model) PersistSession() error {
	return appruntime.SaveSession(appruntime.SessionSaveDeps{
		Runtime:         &m.runtime,
		Cwd:             m.cwd,
		Messages:        m.conv.Messages,
		ReconfigureTool: m.ReconfigureAgentTool,
	})
}

func (m *model) loadSession(id string) error {
	return appruntime.LoadSession(appruntime.SessionLoadDeps{
		Runtime: &m.runtime,
		Cwd:     m.cwd,
		RestoreMessages: func(msgs []core.ChatMessage) {
			m.conv.Messages = msgs
		},
	}, id)
}

func (m *model) InitTaskStorage() {
	appruntime.InitTaskStorage(m.runtime.SessionID)
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	appruntime.RestoreSessionData(&m.runtime, sess, func(msgs []core.ChatMessage) {
		m.conv.Messages = msgs
	})
}

// ============================================================
// Prompt suggestion
// ============================================================

func (m *model) promptSuggestionDeps() input.PromptSuggestionDeps {
	return input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv,
		Runtime:      &m.runtime,
		BuildClient:  m.buildLLMClient,
	}
}

func (m *model) buildPromptSuggestionRequest() (input.PromptSuggestionRequest, bool) {
	return input.BuildPromptSuggestionRequest(m.promptSuggestionDeps())
}

// ============================================================
// conv.MessageRuntime
// ============================================================

func (m *model) CommitMessages() []tea.Cmd { return m.commitMessages() }

func (m *model) ContinueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return conv.DrainAgentOutbox(m.agentSess.agent.Outbox())
}

// ============================================================
// conv.TokenRuntime
// ============================================================

func (m *model) SetTokenCounts(in, out int) {
	m.runtime.InputTokens = in
	m.runtime.OutputTokens = out
}

func (m *model) ClearThinkingOverride() { m.runtime.ThinkingOverride = llm.ThinkingOff }

// ============================================================
// conv.ToolEffectRuntime
// ============================================================

func (m *model) PopToolSideEffect(toolCallID string) any { return tool.PopSideEffect(toolCallID) }

func (m *model) ApplyToolSideEffects(toolName string, sideEffect any) {
	resp, ok := sideEffect.(map[string]any)
	if !ok {
		return
	}
	m.syncBackgroundTaskTrackerFromAgent(toolName, resp)
	switch toolName {
	case "Bash":
		if newCwd := hookResponseString(resp, "cwd"); newCwd != "" {
			m.changeCwd(newCwd)
		}
	case tool.ToolEnterWorktree:
		if worktreePath := hookResponseString(resp, "worktreePath"); worktreePath != "" {
			m.changeCwd(worktreePath)
		}
	case tool.ToolExitWorktree:
		if restoredPath := hookResponseString(resp, "restoredPath"); restoredPath != "" {
			m.changeCwd(restoredPath)
		}
	case "Write", "Edit":
		if filePath := hookResponseString(resp, "filePath"); filePath != "" {
			m.fireFileChanged(filePath, toolName)
			if m.runtime.FileCache != nil {
				m.runtime.FileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := hookResponseString(fileData, "filePath"); filePath != "" && m.runtime.FileCache != nil {
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

func (m *model) FirePostToolHook(tr core.ToolResult, sideEffect any) {
	m.runtime.FirePostToolHook(tr, sideEffect)
}

func (m *model) PersistOverflow(result *core.ToolResult) {
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

// ============================================================
// conv.TurnRuntime
// ============================================================

func (m *model) FireIdleHooks() bool {
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

func (m *model) FireStopFailureHook(err error) {
	m.runtime.FireStopFailureHook(core.LastAssistantChatContent(m.conv.Messages), err)
}

const autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

func (m *model) SaveSession() {
	if err := m.PersistSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}
}

func (m *model) ShouldAutoCompact() bool {
	return kit.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) TriggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = conv.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", kit.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.agentOutput.Spinner.Tick, conv.CompactCmd(m.BuildCompactRequest("", "auto")))
	return tea.Batch(commitCmds...)
}

func (m *model) StopAgentSession() {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
}

func (m *model) HandleCompactResult(msg conv.CompactResultMsg) tea.Cmd {
	shouldContinue := m.conv.Compact.AutoContinue
	if msg.Error != nil {
		m.conv.Compact.Complete(fmt.Sprintf("Compaction could not be completed: %v", msg.Error), true)
		return tea.Batch(m.commitMessages()...)
	}
	m.conv.Compact.Complete(fmt.Sprintf("Condensed %d earlier messages.", msg.OriginalCount), false)
	scrollbackCmds := m.commitAllMessages()
	boundaryStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	boundary := boundaryStyle.Render(fmt.Sprintf("✻ Conversation compacted — %d messages summarized (scroll up for history)", msg.OriginalCount))

	m.conv.Clear()
	m.runtime.ResetTokens()

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

func (m *model) StartPromptSuggestion() tea.Cmd {
	return input.StartPromptSuggestion(m.promptSuggestionDeps())
}

func (m *model) DrainTurnQueues() tea.Cmd {
	for _, drain := range []func() tea.Cmd{m.drainInputQueueToAgent, m.drainCronQueueToAgent, m.drainAsyncHookQueueToAgent, m.drainTaskNotificationsToAgent} {
		if cmd := drain(); cmd != nil {
			return cmd
		}
	}
	return nil
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

// ============================================================
// conv.ProgressRuntime
// ============================================================

func (m *model) HasRunningTasks() bool { return tracker.DefaultStore.HasInProgress() }

// ============================================================
// input.Runtime — overlay selector callbacks
// ============================================================

func (m *model) GetCwd() string                              { return m.cwd }
func (m *model) ReloadPluginBackedState() error               { return m.reloadPluginBackedState() }
func (m *model) ClearCachedInstructions()                     { m.runtime.ClearCachedInstructions() }
func (m *model) SetInputText(text string)                     { m.userInput.Textarea.SetValue(text) }
func (m *model) SetCurrentModel(cm *llm.CurrentModelInfo)     { m.runtime.CurrentModel = cm }
func (m *model) LoadSession(id string) error                  { return m.loadSession(id) }
func (m *model) ResetCommitIndex()                            { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd                 { return m.commitAllMessages() }
func (m *model) SetProviderStatusMessage(msg string)          { m.userInput.Provider.SetStatusMessage(msg) }
func (m *model) RefreshMemoryContext(trigger string)          { m.runtime.RefreshMemoryContext(m.cwd, trigger) }
func (m *model) FireFileChanged(path, toolName string)        { m.fireFileChanged(path, toolName) }
func (m *model) AppendMessage(msg core.ChatMessage)           { m.conv.Append(msg) }
func (m *model) AddNotice(text string)                        { m.conv.AddNotice(text) }

func (m *model) SwitchProvider(p llm.Provider) {
	m.runtime.SwitchProvider(p)
	m.ReconfigureAgentTool()
}

// ============================================================
// input.CommandRuntime + input.SubmitRuntime
// ============================================================

func (m *model) StartExternalEditor(path string) tea.Cmd {
	return kit.StartExternalEditor(path, func(err error) tea.Msg {
		return input.MemoryEditorFinishedMsg{Err: err}
	})
}

func (m *model) SpinnerTickCmd() tea.Cmd { return m.agentOutput.Spinner.Tick }
func (m *model) ResetCronQueue()         { m.systemInput.CronQueue = nil }

func (m *model) FireSessionEnd(reason string) {
	m.runtime.FireSessionEnd(context.Background(), reason)
	if m.systemInput.FileWatcher != nil {
		m.systemInput.FileWatcher.Stop()
	}
}

// ============================================================
// notify.Runtime — task notification injection
// ============================================================

func (m *model) IsInputIdle() bool  { return !m.conv.Stream.Active }
func (m *model) StreamActive() bool { return m.conv.Stream.Active }

func (m *model) InjectTaskNotificationContinuation(item notify.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if m.runtime.LLMProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no provider is connected."})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "A background task completed, but no task notification payload was available."})
		return tea.Batch(m.commitMessages()...)
	}
	for _, ctx := range notify.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(notify.BuildContinuationPrompt(item), nil)
}

// ============================================================
// trigger.Runtime — cron and async hook injection
// ============================================================

func (m *model) AppendNotice(text string) {
	if text == "" {
		return
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: text})
}

func (m *model) InjectCronPrompt(prompt string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt)})
		return tea.Batch(m.commitMessages()...)
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Scheduled task fired"})
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: prompt})
	return m.sendToAgent(prompt, nil)
}

func (m *model) InjectAsyncHookContinuation(item trigger.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: item.Notice})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Async hook requested a follow-up, but no provider is connected."})
		return tea.Batch(m.commitMessages()...)
	}
	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: ctx})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

// ============================================================
// Internal helpers
// ============================================================

func hookResponseString(resp map[string]any, key string) string {
	if value, ok := resp[key].(string); ok {
		return value
	}
	return ""
}


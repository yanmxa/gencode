// Core data types, message commit pipeline, agent session management, and LLM loop configuration.
//
// Agent builder (buildCoreAgent, ensureAgentSession, startAgentLoop) lives here
// because it is Model initialization, not an Update handler.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/app/notify"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool"
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
	"github.com/yanmxa/gencode/internal/tool/fs"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

const defaultWidth = 80

const (
	suggestionSystemPrompt = input.SuggestionSystemPrompt
	suggestionUserPrompt   = input.SuggestionUserPrompt
)

type model struct {
	// ── User Input ──────────────────────────────────────────────────────
	userInput input.Model
	mode      conv.ModalState
	showTasks bool
	tool      conv.ToolExecState

	// ── Agent Input ─────────────────────────────────────────────────────
	agentInput notify.Model

	// ── System Input ────────────────────────────────────────────────────
	systemInput trigger.Model

	// ── Agent Output ────────────────────────────────────────────────────
	conv                 conv.ConversationModel
	agentOutput          conv.Model
	agentSess            *agentSession
	pendingQuestion      *tool.QuestionRequest
	pendingQuestionReply chan *tool.QuestionResponse

	// ── Runtime (shared state: provider, session, permission, plan, config) ──
	runtime appruntime.Model

	// ── Infrastructure ──────────────────────────────────────────────
	cwd           string
	isGit         bool
	width         int
	height        int
	ready         bool
	initialPrompt string
	fileWatcher   *trigger.FileWatcher
	fileCache     *filecache.Cache
}

func (m *model) fireSessionEnd(reason string) {
	m.runtime.FireSessionEnd(context.Background(), reason)
	if m.fileWatcher != nil {
		m.fileWatcher.Stop()
	}
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.agentOutput.Spinner.Tick, m.userInput.MCP.Selector.AutoConnect(), trigger.TriggerCronTickNow(), trigger.StartCronTicker(), trigger.StartAsyncHookTicker(), notify.StartTicker()}
	if m.initialPrompt != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return initialPromptMsg(prompt) })
	}
	return tea.Batch(cmds...)
}

// --- Message commit pipeline ---

func (m *model) commitMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(true)
}

func (m *model) commitAllMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(false)
}

func (m *model) commitMessagesWithCheck(checkReady bool) []tea.Cmd {
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

		if rendered := m.renderSingleMessage(i); rendered != "" {
			printCmds = append(printCmds, tea.Println(rendered))
		}
		m.conv.CommittedCount = i + 1
	}

	// Wrap in tea.Sequence to preserve message ordering.
	// tea.Batch runs commands concurrently, which can scramble the display
	// order when multiple messages are committed at once (e.g., session restore).
	if len(printCmds) > 1 {
		return []tea.Cmd{tea.Sequence(printCmds...)}
	}
	return printCmds
}

// --- Message conversion and LLM loop configuration ---

// reconfigureAgentTool updates the agent tool with the current session/provider state.
func (m *model) reconfigureAgentTool() {
	if m.runtime.LLMProvider != nil {
		m.ensureMemoryContextLoaded()
		configureAgentTool(m.runtime.LLMProvider, m.cwd, m.getModelID(), m.runtime.HookEngine, m.runtime.SessionStore, m.runtime.SessionID,
			m.agentToolOpts()...)
	}
}

func (m *model) agentToolOpts() []agentToolOption {
	opts := []agentToolOption{
		withAgentContext(m.runtime.CachedUserInstructions, m.runtime.CachedProjectInstructions, m.isGit),
	}
	if mcp.DefaultRegistry != nil {
		opts = append(opts, withAgentMCP(mcp.DefaultRegistry.GetToolSchemas, mcp.DefaultRegistry))
	}
	return opts
}

func (m *model) ensureMemoryContextLoaded() {
	if m.runtime.CachedUserInstructions != "" || m.runtime.CachedProjectInstructions != "" {
		return
	}
	m.runtime.RefreshMemoryContext(m.cwd, "session_start")
}

func (m *model) effectiveThinkingLevel() llm.ThinkingLevel {
	return m.runtime.EffectiveThinkingLevel()
}

func (m model) getModelID() string {
	return m.runtime.GetModelID()
}

func (m *model) getEffectiveInputLimit() int {
	return kit.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return kit.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens)
}

func (m *model) getContextUsagePercent() float64 {
	return kit.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) shouldAutoCompact() bool {
	return kit.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func formatAsyncHookContinuationContext(result hook.AsyncHookResult, reason string) string {
	return fmt.Sprintf(
		"<background-hook-result>\nstatus: blocked\nevent: %s\nhook_type: %s\nhook_source: %s\nhook_name: %s\nreason: %s\ninstruction: Re-evaluate the plan before any further model or tool action.\n</background-hook-result>",
		result.Event,
		result.HookType,
		result.HookSource,
		result.HookName,
		reason,
	)
}

// --- Agent session management ---
// Agent builder (buildCoreAgent, ensureAgentSession, startAgentLoop) lives
// in model.go because it is Model initialization, not an Update handler.

// agentSession holds the running core.Agent and its supporting infrastructure.
type agentSession struct {
	agent              core.Agent
	permBridge         *conv.PermissionBridge
	cancel             context.CancelFunc
	pendingPermRequest *conv.PermBridgeRequest
}

// buildCoreAgent creates a core.Agent and permissionBridge from the model's
// current state. The agent is not started — call startAgentLoop() for that.
func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.runtime.LLMProvider == nil {
		return nil, errNoProvider
	}

	// LLM — wraps the current provider as core.LLM
	client := llm.NewClient(m.runtime.LLMProvider, m.getModelID(), m.getMaxTokens())
	client.SetThinking(m.effectiveThinkingLevel())

	// System prompt — build layered core.System directly
	c := m.buildLoopClient()
	sys := m.buildLoopSystem(nil, c)

	// Tools — adapt legacy tool registry to core.Tools
	schemas := m.buildLoopToolSet().Tools()
	tools := tool.AdaptToolRegistry(schemas, func() string { return m.cwd })

	// MCP tools — add MCP tool executors so core.Agent can execute them
	if mcp.DefaultRegistry != nil {
		mcpCaller := mcp.NewCaller(mcp.DefaultRegistry)
		for _, t := range mcp.AsCoreTools(schemas, mcpCaller) {
			tools.Add(t)
		}
	}

	// Permission bridge — blocking PermissionFunc with TUI approval
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

	// Wrap tools with permission decorator
	permTools := tool.WithPermission(tools, permBridge.PermissionFunc())

	ag := core.NewAgent(core.Config{
		ID:     "main",
		LLM:    client,
		System: sys,
		Tools:  permTools,
		CWD:    m.cwd,
	})

	return &agentSession{
		agent:      ag,
		permBridge: permBridge,
	}, nil
}

// startAgentLoop starts the core.Agent in a background goroutine and returns
// tea.Cmds for draining the outbox and polling the permission bridge.
func (m *model) startAgentLoop(sess *agentSession) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	sess.cancel = cancel

	// Start agent.Run in background
	go func() {
		_ = sess.agent.Run(ctx)
	}()

	// Return commands that drain the outbox and poll the permission bridge
	return tea.Batch(
		conv.DrainAgentOutbox(sess.agent.Outbox()),
		conv.PollPermBridge(sess.permBridge),
	)
}

// stopAgentLoop gracefully stops the running agent.
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
	// Send stop signal if inbox is still open
	if sess.agent != nil {
		select {
		case sess.agent.Inbox() <- core.Message{Signal: core.SigStop}:
		default:
		}
	}
}

// ensureAgentSession lazily creates and starts the core.Agent session.
func (m *model) ensureAgentSession() error {
	if m.agentSess != nil {
		return nil
	}
	sess, err := m.buildCoreAgent()
	if err != nil {
		return err
	}
	m.agentSess = sess

	// Restore existing conversation history into the agent
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

var errNoProvider = providerRequiredError("no LLM provider configured")

type providerRequiredError string

func (e providerRequiredError) Error() string { return string(e) }

// --- LLM loop configuration and agent tool wiring ---

func (m *model) buildLoopClient() *llm.Client {
	c := llm.NewClient(m.runtime.LLMProvider, m.getModelID(), m.getMaxTokens())
	c.SetThinking(m.effectiveThinkingLevel())
	return c
}

func (m *model) buildPromptSuggestionRequest() (input.PromptSuggestionRequest, bool) {
	return input.BuildPromptSuggestionRequest(input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv,
		Runtime:      &m.runtime,
		BuildClient:  m.buildLoopClient,
	})
}

func (m *model) startPromptSuggestion() tea.Cmd {
	return input.StartPromptSuggestion(input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv,
		Runtime:      &m.runtime,
		BuildClient:  m.buildLoopClient,
	})
}

func (m *model) buildCompactRequest(focus, trigger string) conv.CompactRequest {
	return conv.CompactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLoopClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.runtime.SessionSummary,
		Focus:          focus,
		HookEngine:     m.runtime.HookEngine,
		Trigger:        trigger,
	}
}

func (m *model) saveSession() error {
	return appruntime.SaveSession(appruntime.SessionSaveDeps{
		Runtime:         &m.runtime,
		Cwd:             m.cwd,
		Messages:        m.conv.Messages,
		ReconfigureTool: m.reconfigureAgentTool,
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

func (m *model) initTaskStorage() {
	appruntime.InitTaskStorage(m.runtime.SessionID)
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	appruntime.RestoreSessionData(&m.runtime, sess, func(msgs []core.ChatMessage) {
		m.conv.Messages = msgs
	})
}

func (m *model) buildLoopSystem(extra []string, loopClient *llm.Client) core.System {
	providerName := ""
	modelID := ""
	if loopClient != nil {
		modelID = loopClient.ModelID()
		providerName = loopClient.Name()
	}
	return system.Build(system.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.runtime.PlanEnabled,
		UserInstructions:    m.runtime.CachedUserInstructions,
		ProjectInstructions: m.runtime.CachedProjectInstructions,
		SessionSummary:      m.buildSessionSummaryBlock(),
		Skills:              m.buildLoopSkillsSection(),
		Agents:              m.buildLoopAgentsSection(),
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               m.buildLoopExtra(extra),
	})
}

func (m *model) buildLoopToolSet() *tool.Set {
	return &tool.Set{
		Disabled: m.runtime.DisabledTools,
		PlanMode: m.runtime.PlanEnabled,
		MCP:      m.buildMCPToolsGetter(),
	}
}

func (m *model) buildLoopExtra(extra []string) []string {
	allExtra := append([]string{}, extra...)
	if coordinator := buildCoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.userInput.Skill.ActiveInvocation)
	}
	return allExtra
}

func buildCoordinatorGuidance() string {
	return system.CoordinatorGuidance()
}

func (m *model) buildSessionSummaryBlock() string {
	if m.runtime.SessionSummary == "" {
		return ""
	}
	return fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.runtime.SessionSummary)
}

func (m *model) buildLoopSkillsSection() string {
	if skill.DefaultRegistry == nil {
		return ""
	}
	return skill.DefaultRegistry.GetSkillsSection()
}

func (m *model) buildLoopAgentsSection() string {
	if subagent.DefaultRegistry == nil {
		return ""
	}
	return subagent.DefaultRegistry.GetAgentsSection()
}

func (m *model) buildMCPToolsGetter() func() []core.ToolSchema {
	if mcp.DefaultRegistry == nil {
		return nil
	}
	return mcp.DefaultRegistry.GetToolSchemas
}

type agentToolOption func(*subagent.Executor)

func configureAgentTool(llmProvider llm.Provider, cwd string, modelID string, hookEngine *hook.Engine, sessionStore *session.Store, parentSessionID string, opts ...agentToolOption) {
	executor := subagent.NewExecutor(llmProvider, cwd, modelID, hookEngine)
	if sessionStore != nil && parentSessionID != "" {
		executor.SetSessionStore(sessionStore, parentSessionID)
	}
	for _, opt := range opts {
		opt(executor)
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

func withAgentContext(userInstructions, projectInstructions string, isGit bool) agentToolOption {
	return func(e *subagent.Executor) {
		e.SetContext(userInstructions, projectInstructions, isGit)
	}
}

func withAgentMCP(getter func() []core.ToolSchema, registry *mcp.Registry) agentToolOption {
	return func(e *subagent.Executor) {
		e.SetMCP(getter, registry)
	}
}

// --- Infrastructure initialization and model construction ---

var appCwd string

func initInfrastructure() error {
	appCwd, _ = os.Getwd()

	llm.Initialize()
	initExtensions(appCwd)
	setting.Initialize(appCwd)
	if err := initTools(appCwd); err != nil {
		return err
	}
	session.Initialize(appCwd)

	hookSettings := setting.DefaultSetup
	plugin.MergePluginHooksIntoSettings(hookSettings)
	hook.Initialize(hook.InitializeConfig{
		Settings:       hookSettings,
		SessionID:      session.DefaultSetup.SessionID,
		CWD:            appCwd,
		TranscriptPath: session.DefaultSetup.TranscriptPath(),
		Provider:       llm.DefaultSetup.Provider,
		ModelID:        llm.DefaultSetup.ModelID(),
		EnvProvider:    plugin.PluginEnv,
	})

	return nil
}

func initTools(cwd string) error {
	orchestration.DefaultStore.Reset()
	cron.DefaultStore.Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return fmt.Errorf("failed to load scheduled tasks: %w", err)
	}
	fs.SetEnvProvider(plugin.PluginEnv)
	return nil
}

func newModel(opts setting.RunOptions) (*model, error) {
	base := newBaseModel()
	m := &base
	notify.InstallCompletionObserver(m.agentInput.Notifications)
	m.configureAsyncHookCallback()
	m.ensureMemoryContextLoaded()
	m.reconfigureAgentTool()
	m.initTaskStorage()
	if err := m.applyRunOptions(opts); err != nil {
		return nil, err
	}
	return m, nil
}

func newBaseModel() model {
	progressHub := conv.NewProgressHub(100)

	userInput := input.New(appCwd, defaultWidth, commandSuggestionMatcher())
	userInput.Agent = input.NewAgentSelector(&agentRegistryAdapter{subagent.DefaultRegistry})
	userInput.Search = input.NewSearchSelector()
	userInput.Skill = input.SkillState{Selector: input.NewSkillSelector(skill.DefaultRegistry)}
	userInput.Session = input.SessionState{Selector: input.NewSessionSelector()}
	userInput.Memory = input.MemoryState{Selector: input.NewMemorySelector()}
	userInput.Approval = input.NewApproval()
	userInput.MCP = input.MCPState{Selector: input.NewMCPSelector(mcp.DefaultRegistry)}
	userInput.Plugin = input.NewPluginSelector(plugin.DefaultRegistry)
	userInput.Provider = input.ProviderState{Selector: input.NewProviderSelector()}
	userInput.Tool = input.NewToolSelector(setting.GetDisabledToolsAt, setting.UpdateDisabledToolsAt)

	return model{
		userInput:   userInput,
		agentOutput: conv.New(defaultWidth, progressHub),
		conv:        conv.NewConversation(),
		cwd:         appCwd,
		showTasks:   true,

		runtime: appruntime.Model{
			OperationMode:      setting.ModeNormal,
			SessionPermissions: setting.NewSessionPermissions(),
			DisabledTools:      setting.GetDisabledTools(),

			LLMProvider:   llm.DefaultSetup.Provider,
			ProviderStore: llm.DefaultSetup.Store,
			CurrentModel:  llm.DefaultSetup.CurrentModel,

			SessionStore: session.DefaultSetup.Store,
			SessionID:    session.DefaultSetup.SessionID,

			Settings:   setting.DefaultSetup,
			HookEngine: hook.DefaultEngine,
		},

		mode:        newModeState(),
		tool:        conv.ToolExecState{},
		isGit:       setting.IsGitRepo(appCwd),
		systemInput: trigger.New(hook.DefaultEngine),
		fileWatcher: trigger.NewFileWatcher(hook.DefaultEngine, nil),
		agentInput:  notify.New(),
		fileCache:   filecache.New(),
	}
}

func commandSuggestionMatcher() func(string) []suggest.Suggestion {
	return func(query string) []suggest.Suggestion {
		cmds := command.GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	}
}

func newModeState() conv.ModalState {
	return conv.ModalState{
		PlanApproval: conv.NewPlanPrompt(),
		PlanEntry:    conv.NewEnterPlanPrompt(),
		Question:     conv.NewQuestionPrompt(),
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
	m.reconfigureAgentTool()

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

// --- Extension initialization helpers ---

func initExtensions(cwd string) {
	if err := plugin.Initialize(context.Background(), cwd); err != nil {
		log.Logger().Warn("Failed to initialize plugin", zap.Error(err))
	}
	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill", zap.Error(err))
	}
	command.SetDynamicInfoProviders(skillCommandInfos)
	if err := command.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize command", zap.Error(err))
	}
	if err := subagent.Initialize(cwd, pluginAgentPaths); err != nil {
		log.Logger().Warn("Failed to initialize subagent", zap.Error(err))
	}
	if err := mcp.Initialize(cwd, pluginMCPServers); err != nil {
		log.Logger().Warn("Failed to initialize mcp", zap.Error(err))
	}
}

func pluginAgentPaths() []subagent.PluginAgentPath {
	pPaths := plugin.GetPluginAgentPaths()
	paths := make([]subagent.PluginAgentPath, len(pPaths))
	for i, p := range pPaths {
		paths[i] = subagent.PluginAgentPath{
			Path:      p.Path,
			Namespace: p.Namespace,
		}
	}
	return paths
}

func pluginMCPServers() []mcp.PluginServer {
	pServers := plugin.GetPluginMCPServers()
	servers := make([]mcp.PluginServer, len(pServers))
	for i, s := range pServers {
		servers[i] = mcp.PluginServer{
			Name:    s.Name,
			Type:    string(s.Config.Type),
			Command: s.Config.Command,
			Args:    append([]string(nil), s.Config.Args...),
			Env:     s.Config.Env,
			URL:     s.Config.URL,
			Headers: s.Config.Headers,
			Scope:   string(s.Scope),
		}
	}
	return servers
}

type agentRegistryAdapter struct {
	reg *subagent.Registry
}

func (a *agentRegistryAdapter) ListConfigs() []input.AgentConfigInfo {
	configs := a.reg.ListConfigs()
	out := make([]input.AgentConfigInfo, len(configs))
	for i, cfg := range configs {
		var tools []string
		if cfg.Tools != nil {
			tools = []string(cfg.Tools)
		}
		out[i] = input.AgentConfigInfo{
			Name:           cfg.Name,
			Description:    cfg.Description,
			Model:          cfg.Model,
			PermissionMode: string(cfg.PermissionMode),
			Tools:          tools,
			SourceFile:     cfg.SourceFile,
		}
	}
	return out
}

func (a *agentRegistryAdapter) GetDisabledAt(userLevel bool) map[string]bool {
	return a.reg.GetDisabledAt(userLevel)
}

func (a *agentRegistryAdapter) SetEnabled(name string, enabled bool, userLevel bool) error {
	return a.reg.SetEnabled(name, enabled, userLevel)
}

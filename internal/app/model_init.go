package app

import (
	"context"
	"fmt"
	"path/filepath"

	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/agentui"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/app/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/pluginui"
	"github.com/yanmxa/gencode/internal/app/progress"
	"github.com/yanmxa/gencode/internal/app/providerui"
	"github.com/yanmxa/gencode/internal/app/searchui"
	"github.com/yanmxa/gencode/internal/app/sessionui"
	"github.com/yanmxa/gencode/internal/app/skillui"
	"github.com/yanmxa/gencode/internal/app/suggest"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/prompt"
	"github.com/yanmxa/gencode/internal/cron"
	appcommand "github.com/yanmxa/gencode/internal/ext/command"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
	"github.com/yanmxa/gencode/internal/tool/fs"
	"github.com/yanmxa/gencode/internal/tool/web"
	"github.com/yanmxa/gencode/internal/util/filecache"
	"github.com/yanmxa/gencode/internal/util/log"
)

type modelInfra struct {
	store            *llm.Store
	llmProvider      llm.Provider
	currentModel     *llm.CurrentModelInfo
	settings         *config.Settings
	hookEngine       *hook.Engine
	sessionStore     *session.Store
	notifications    *appagent.NotificationQueue
	initialSessionID string
}

func initializeModelInfra(cwd string) (modelInfra, error) {
	orchestration.DefaultStore.Reset()
	cron.DefaultStore.Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return modelInfra{}, fmt.Errorf("failed to load scheduled tasks: %w", err)
	}

	store, llmProvider, currentModel := initializeProvider()
	initializeRegistries(cwd)
	settings := loadSettingsForCwd(cwd)

	// Wire injected dependencies so tool layer doesn't import upper layers
	if store != nil {
		web.SetSearchProviderGetter(store.GetSearchProvider)
	}
	fs.SetBashEnvProvider(plugin.PluginEnv)

	sessionID := session.NewSessionID()

	var transcriptPath string
	sessionStore, err := session.NewStore(cwd)
	if err != nil {
		log.Logger().Warn("session store initialization failed, sessions will not be persisted", zap.Error(err))
	}
	if sessionStore != nil {
		transcriptPath = sessionStore.SessionPath(sessionID)
	}
	notifications := appagent.NewNotificationQueue()
	hookEngine := hook.NewEngine(settings, sessionID, cwd, transcriptPath)
	modelID := ""
	if currentModel != nil {
		modelID = currentModel.ModelID
	}
	hookEngine.SetLLMCompleter(buildLLMCompleter(llmProvider), modelID)
	hookEngine.SetAgentRunner(newHookAgentRunner(llmProvider, settings, cwd, config.IsGitRepo(cwd), mcp.DefaultRegistry, modelID))
	hookEngine.SetEnvProvider(plugin.PluginEnv)
	installHookBridges(hookEngine, notifications)

	return modelInfra{
		store:            store,
		llmProvider:      llmProvider,
		currentModel:     currentModel,
		settings:         settings,
		hookEngine:       hookEngine,
		sessionStore:     sessionStore,
		notifications:    notifications,
		initialSessionID: sessionID,
	}, nil
}

func newBaseModel(cwd string, infra modelInfra) model {
	progressHub := progress.NewHub(100)

	return model{
		userInput:  appuser.New(cwd, defaultWidth, commandSuggestionMatcher()),
		agentOutput: appoutput.New(defaultWidth, progressHub),
		conv:   appconv.New(),
		cwd:    cwd,

		provider: newProviderState(infra),
		session:  newSessionState(infra),
		skill:    newSkillState(),
		memory:   newMemoryState(),
		mode:     newModeState(),
		tool:     newToolState(),
		mcp:      newMCPState(),
		plugin:   newPluginState(),
		agent:    newAgentState(),
		search:   newSearchState(),
		approval: appapproval.New(),

		queueSelectIdx: -1,

		showTasks: true,
		isGit:     config.IsGitRepo(cwd),

		systemInput: appsystem.New(),
		settings:    infra.settings,
		hookEngine:  infra.hookEngine,
		fileWatcher: newFileWatcher(infra.hookEngine, nil),
		agentInput:  appagent.State{Notifications: infra.notifications},
		fileCache:   filecache.New(),
	}
}

func commandSuggestionMatcher() func(string) []suggest.Suggestion {
	return func(query string) []suggest.Suggestion {
		cmds := appcommand.GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	}
}

func newProviderState(infra modelInfra) providerui.State {
	return providerui.State{
		LLM:          infra.llmProvider,
		Store:        infra.store,
		CurrentModel: infra.currentModel,
		Selector:     providerui.New(),
	}
}

func newSessionState(infra modelInfra) sessionui.State {
	return sessionui.State{
		Selector:  sessionui.New(),
		Store:     infra.sessionStore,
		CurrentID: infra.initialSessionID,
	}
}

func newSkillState() skillui.State {
	return skillui.State{Selector: skillui.New()}
}

func newMemoryState() appmemory.State {
	return appmemory.State{Selector: appmemory.New()}
}

func newModeState() appmode.State {
	return appmode.State{
		Operation:          config.ModeNormal,
		SessionPermissions: config.NewSessionPermissions(),
		DisabledTools:      config.GetDisabledTools(),
		PlanApproval:       appmode.NewPlanPrompt(),
		PlanEntry:          appmode.NewEnterPlanPrompt(),
		Question:           appmode.NewQuestionPrompt(),
	}
}

func newToolState() toolui.State {
	return toolui.State{Selector: toolui.New()}
}

func newMCPState() mcpui.State {
	return mcpui.State{Selector: mcpui.New(), Registry: mcp.DefaultRegistry}
}

func newPluginState() pluginui.State {
	return pluginui.State{Selector: pluginui.New()}
}

func newAgentState() agentui.State {
	return agentui.State{Selector: agentui.New()}
}

func newSearchState() searchui.State {
	return searchui.State{Selector: searchui.New()}
}

func (m *model) applyRunOptions(opts config.RunOptions) error {
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
		if err := m.applyContinueOption(opts.Fork); err != nil {
			return err
		}
	}

	if opts.Resume {
		if err := m.applyResumeOption(opts.ResumeID, opts.Fork); err != nil {
			return err
		}
	}

	return nil
}

func (m *model) reloadPluginBackedState() error {
	if err := skill.Initialize(m.cwd); err != nil {
		return fmt.Errorf("failed to reload skill registry: %w", err)
	}
	appcommand.SetDynamicInfoProviders(skillCommandInfos)
	if err := appcommand.Initialize(m.cwd); err != nil {
		return fmt.Errorf("failed to reload custom commands: %w", err)
	}
	if err := subagent.Initialize(m.cwd); err != nil {
		return fmt.Errorf("failed to reload agent registry: %w", err)
	}
	if err := mcp.Initialize(m.cwd); err != nil {
		return fmt.Errorf("failed to reload MCP registry: %w", err)
	}
	m.mcp.Registry = mcp.DefaultRegistry

	settings := loadSettings()
	m.settings = settings
	if m.hookEngine != nil {
		m.hookEngine.SetSettings(settings)
		m.hookEngine.SetAgentRunner(newHookAgentRunner(m.provider.LLM, settings, m.cwd, m.isGit, m.mcp.Registry, m.getModelID()))
	}
	m.reconfigureAgentTool()

	return nil
}

func (m *model) enablePlanMode(prompt string) error {
	m.mode.Enabled = true
	m.mode.Task = prompt
	m.mode.Operation = config.ModePlan

	planStore, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.mode.Store = planStore
	return nil
}

func (m *model) applyContinueOption(fork bool) error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.session.Store = sessionStore

	sess, err := sessionStore.GetLatest()
	if err != nil {
		return fmt.Errorf("no previous session to continue: %w", err)
	}

	if fork {
		forked, err := sessionStore.Fork(sess.Metadata.ID)
		if err != nil {
			return fmt.Errorf("failed to fork session: %w", err)
		}
		sess = forked
	}

	m.restoreSessionData(sess)
	return nil
}

func (m *model) applyResumeOption(resumeID string, fork bool) error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.session.Store = sessionStore

	if resumeID != "" {
		sess, err := sessionStore.Load(resumeID)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", resumeID, err)
		}
		if fork {
			forked, err := sessionStore.Fork(sess.Metadata.ID)
			if err != nil {
				return fmt.Errorf("failed to fork session: %w", err)
			}
			sess = forked
		}
		m.restoreSessionData(sess)
		return nil
	}

	m.session.PendingSelector = true
	m.session.PendingFork = fork
	return nil
}

// buildLLMCompleter wraps a provider into an hook.LLMCompleter closure.
// The closure owns client construction and streaming, keeping the hooks
// engine free from direct provider dependencies.
func buildLLMCompleter(p llm.Provider) hook.LLMCompleter {
	if p == nil {
		return nil
	}
	return func(ctx context.Context, systemPrompt, userMessage, model string) (string, error) {
		c := llm.NewClient(p, model, 0)
		resp, err := c.Complete(ctx, systemPrompt, []core.Message{{
			Role:    core.RoleUser,
			Content: userMessage,
		}}, 4096)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}

func (m *model) buildLoopClient() *llm.Client {
	llm := llm.NewClient(m.provider.LLM, m.getModelID(), m.getMaxTokens())
	llm.SetThinking(m.effectiveThinkingLevel())
	return llm
}

func (m *model) buildLoopSystem(extra []string, loopClient *llm.Client) core.System {
	providerName := ""
	modelID := ""
	if loopClient != nil {
		modelID = loopClient.ModelID()
		providerName = loopClient.Name()
	}
	return prompt.Build(prompt.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.mode.Enabled,
		UserInstructions:    m.memory.CachedUser,
		ProjectInstructions: m.memory.CachedProject,
		SessionSummary:      m.buildSessionSummaryBlock(),
		Skills:              m.buildLoopSkillsSection(),
		Agents:              m.buildLoopAgentsSection(),
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               m.buildLoopExtra(extra),
	})
}

func (m *model) buildLoopToolSet() *tool.Set {
	return &tool.Set{
		Disabled: m.mode.DisabledTools,
		PlanMode: m.mode.Enabled,
		MCP:      m.buildMCPToolsGetter(),
	}
}

func (m *model) buildLoopExtra(extra []string) []string {
	allExtra := append([]string{}, extra...)
	if coordinator := buildCoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.skill.ActiveInvocation)
	}
	if reminder := m.buildTaskReminder(); reminder != "" {
		allExtra = append(allExtra, reminder)
	}
	return allExtra
}

func buildCoordinatorGuidance() string {
	return prompt.CoordinatorGuidance()
}

func (m *model) buildSessionSummaryBlock() string {
	if m.session.Summary == "" {
		return ""
	}
	return fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.session.Summary)
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
	if m.mcp.Registry == nil {
		return nil
	}
	return m.mcp.Registry.GetToolSchemas
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

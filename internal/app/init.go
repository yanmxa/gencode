package app

import (
	"context"
	"fmt"
	"path/filepath"

	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/user/agentui"
	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	appmodal "github.com/yanmxa/gencode/internal/app/modal"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/output/progress"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/app/user/searchui"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/app/user/skillui"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/cron"
	appcommand "github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool/fs"
	"github.com/yanmxa/gencode/internal/tool/web"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/log"
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

func initInfra(cwd string) (modelInfra, error) {
	orchestration.DefaultStore.Reset()
	cron.DefaultStore.Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return modelInfra{}, fmt.Errorf("failed to load scheduled tasks: %w", err)
	}

	store, llmProvider, currentModel := initLLM()
	initExt(cwd)
	settings := initSettings(cwd)

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
	hookEngine.SetAgentRunner(NewHookAgentRunner(llmProvider, settings, cwd, config.IsGitRepo(cwd), mcp.DefaultRegistry, modelID))
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
		userInput:   appuser.New(cwd, defaultWidth, commandSuggestionMatcher()),
		agentOutput: appoutput.New(defaultWidth, progressHub),
		conv:        appconv.New(),
		cwd:         cwd,
		showTasks:   true,

		operationMode:      config.ModeNormal,
		sessionPermissions: config.NewSessionPermissions(),
		disabledTools:      config.GetDisabledTools(),

		llmProvider:   infra.llmProvider,
		providerStore: infra.store,
		currentModel:  infra.currentModel,

		sessionStore: infra.sessionStore,
		sessionID:    infra.initialSessionID,

		provider: newProviderState(),
		session:  newSessionState(),
		skill:    newSkillState(),
		memory:   newMemoryState(),
		mode:     newModeState(),
		tool:     newToolState(),
		mcp:      newMCPState(),
		plugin:   newPluginState(),
		agent:    newAgentState(),
		search:   newSearchState(),
		approval: appapproval.New(),
		isGit:    config.IsGitRepo(cwd),

		systemInput: appsystem.New(),
		settings:    infra.settings,
		hookEngine:  infra.hookEngine,
		fileWatcher: appsystem.NewFileWatcher(infra.hookEngine, nil),
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

func newProviderState() providerui.State {
	return providerui.State{
		Selector: providerui.New(),
	}
}

func newSessionState() sessionui.State {
	return sessionui.State{
		Selector: sessionui.New(),
	}
}

func newSkillState() skillui.State {
	return skillui.State{Selector: skillui.New(skill.DefaultRegistry)}
}

func newMemoryState() appmemory.State {
	return appmemory.State{Selector: appmemory.New()}
}

func newModeState() appmodal.State {
	return appmodal.State{
		PlanApproval: appmodal.NewPlanPrompt(),
		PlanEntry:    appmodal.NewEnterPlanPrompt(),
		Question:     appmodal.NewQuestionPrompt(),
	}
}

func newToolState() toolui.State {
	return toolui.State{Selector: toolui.New()}
}

func newMCPState() mcpui.State {
	return mcpui.State{Selector: mcpui.New(mcp.DefaultRegistry)}
}

func newPluginState() pluginui.Model {
	return pluginui.New(plugin.DefaultRegistry)
}

func newAgentState() agentui.Model {
	return agentui.New(subagent.DefaultRegistry)
}

func newSearchState() searchui.Model {
	return searchui.New()
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

	settings := initSettings(m.cwd)
	m.settings = settings
	if m.hookEngine != nil {
		m.hookEngine.SetSettings(settings)
		m.hookEngine.SetAgentRunner(NewHookAgentRunner(m.llmProvider, settings, m.cwd, m.isGit, mcp.DefaultRegistry, m.getModelID()))
	}
	m.reconfigureAgentTool()

	return nil
}

func (m *model) enablePlanMode(prompt string) error {
	m.planEnabled = true
	m.planTask = prompt
	m.operationMode = config.ModePlan

	planStore, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.planStore = planStore
	return nil
}

func (m *model) applyContinueOption() error {
	sessionStore, err := session.NewStore(m.cwd)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.sessionStore = sessionStore

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
	m.sessionStore = sessionStore

	if resumeID != "" {
		sess, err := sessionStore.Load(resumeID)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", resumeID, err)
		}
		m.restoreSessionData(sess)
		return nil
	}

	m.session.PendingSelector = true
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


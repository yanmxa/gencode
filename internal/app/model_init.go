package app

import (
	"context"
	"fmt"
	"path/filepath"

	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agentinput"
	"github.com/yanmxa/gencode/internal/app/agentui"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/pluginui"
	"github.com/yanmxa/gencode/internal/app/providerui"
	"github.com/yanmxa/gencode/internal/app/searchui"
	"github.com/yanmxa/gencode/internal/app/sessionui"
	"github.com/yanmxa/gencode/internal/app/skillui"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/cron"
	appcommand "github.com/yanmxa/gencode/internal/ext/command"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/util/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool/fs"
	"github.com/yanmxa/gencode/internal/tool/web"
	"github.com/yanmxa/gencode/internal/app/progress"
	"github.com/yanmxa/gencode/internal/app/suggest"
)

type modelInfra struct {
	store             *llm.Store
	llmProvider       llm.LLMProvider
	currentModel      *llm.CurrentModelInfo
	settings          *config.Settings
	hookEngine        *hook.Engine
	sessionStore      *session.Store
	notifications *appagent.NotificationQueue
	initialSessionID  string
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
		store:             store,
		llmProvider:       llmProvider,
		currentModel:      currentModel,
		settings:          settings,
		hookEngine:        hookEngine,
		sessionStore:      sessionStore,
		taskNotifications: taskNotifications,
		initialSessionID:  sessionID,
	}, nil
}

func newBaseModel(cwd string, infra modelInfra) model {
	progressHub := progress.NewHub(100)

	return model{
		input:  appinput.New(cwd, defaultWidth, commandSuggestionMatcher()),
		output: appoutput.New(defaultWidth, progressHub),
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

		systemInput:       appsystem.New(),
		settings:          infra.settings,
		hookEngine:        infra.hookEngine,
		fileWatcher:       newFileWatcher(infra.hookEngine, nil),
		taskNotifications: infra.taskNotifications,
		fileCache:         filecache.New(),
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
func buildLLMCompleter(p llm.LLMProvider) hook.LLMCompleter {
	if p == nil {
		return nil
	}
	return func(ctx context.Context, systemPrompt, userMessage, model string) (string, error) {
		c := llm.NewLLM(p, model, 0)
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

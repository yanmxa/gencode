package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/app/agentui"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appcommand "github.com/yanmxa/gencode/internal/app/command"
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
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tracker"
	"github.com/yanmxa/gencode/internal/ui/progress"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

type modelInfra struct {
	store             *provider.Store
	llmProvider       provider.LLMProvider
	currentModel      *provider.CurrentModelInfo
	settings          *config.Settings
	hookEngine        *hooks.Engine
	sessionStore      *session.Store
	taskNotifications *taskNotificationQueue
	initialSessionID  string
}

func initializeModelInfra(cwd string) (modelInfra, error) {
	cron.DefaultStore = cron.NewStore()
	orchestration.DefaultStore.Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return modelInfra{}, fmt.Errorf("failed to load scheduled tasks: %w", err)
	}

	store, llmProvider, currentModel := initializeProvider()
	initializeRegistries(cwd)
	settings := loadSettingsForCwd(cwd)

	sessionID := session.NewSessionID()

	var transcriptPath string
	sessionStore, _ := session.NewStore(cwd)
	if sessionStore != nil {
		transcriptPath = sessionStore.SessionPath(sessionID)
	}
	taskNotifications := newTaskNotificationQueue()
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, transcriptPath)
	modelID := ""
	if currentModel != nil {
		modelID = currentModel.ModelID
	}
	hookEngine.SetLLMProvider(llmProvider, modelID)
	hookEngine.SetAgentRunner(newHookAgentRunner(llmProvider, settings, cwd, config.IsGitRepo(cwd), mcp.DefaultRegistry))
	installHookBridges(hookEngine, taskNotifications)

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

		settings:          infra.settings,
		hookEngine:        infra.hookEngine,
		fileWatcher:       newFileWatcher(infra.hookEngine, nil),
		asyncHookQueue:    newAsyncHookQueue(),
		taskNotifications: infra.taskNotifications,
		loop:              &runtime.Loop{},
		runtime:           newConversationRuntime(),
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
	if err := appcommand.Initialize(m.cwd); err != nil {
		return fmt.Errorf("failed to reload custom commands: %w", err)
	}
	if err := agent.Initialize(m.cwd); err != nil {
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
		m.hookEngine.SetAgentRunner(newHookAgentRunner(m.provider.LLM, settings, m.cwd, m.isGit, m.mcp.Registry))
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

func (m *model) initializeTaskStorageFromEnv() {
	if taskListID := os.Getenv("GEN_TASK_LIST_ID"); taskListID != "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			dir := filepath.Join(homeDir, ".gen", "tasks", taskListID)
			tracker.DefaultStore.SetStorageDir(dir)
			_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
		}
	}
}

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	appmodal "github.com/yanmxa/gencode/internal/app/output/modal"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/output/progress"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
	appuser "github.com/yanmxa/gencode/internal/app/user"

	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool/fs"
)

// appCwd holds the working directory, initialized by initInfrastructure().
// Other singletons live in their domain packages:
//   llm.DefaultSetup, session.DefaultSetup, setting.DefaultSetup, hook.DefaultEngine
var appCwd string

func initInfrastructure() error {
	appCwd, _ = os.Getwd()

	// 1. LLM — no deps
	llm.Initialize()

	// 2. Extensions — plugin, skill, command, subagent, MCP
	initExtensions(appCwd)

	// 3. Settings
	setting.Initialize(appCwd)

	// 4. Tools — orchestration, cron, cross-cutting wiring
	if err := initTools(appCwd); err != nil {
		return err
	}

	// 5. Session
	session.Initialize(appCwd)

	// 6. Hook engine
	hook.Initialize(appCwd)

	return nil
}

func initTools(cwd string) error {
	orchestration.DefaultStore.Reset()
	cron.DefaultStore.Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return fmt.Errorf("failed to load scheduled tasks: %w", err)
	}
	// plugin env vars (e.g., GEN_PLUGIN_<name>_ROOT) injected into Bash child processes
	fs.SetEnvProvider(plugin.PluginEnv)
	return nil
}

func newModel(opts setting.RunOptions) (*model, error) {
	base := newBaseModel()
	m := &base
	// TODO: refactor hook bridges to avoid global side-effect registration
	installHookBridges(m.hookEngine, m.agentInput.Notifications)
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
	progressHub := progress.NewHub(100)

	userInput := appuser.New(appCwd, defaultWidth, commandSuggestionMatcher())
	userInput.Agent = appuser.NewAgentSelector(subagent.DefaultRegistry)
	userInput.Search = appuser.NewSearchSelector()
	userInput.Skill = appuser.SkillState{Selector: appuser.NewSkillSelector(skill.DefaultRegistry)}

	return model{
		userInput:   userInput,
		agentOutput: appoutput.New(defaultWidth, progressHub),
		conv:        appconv.New(),
		cwd:         appCwd,
		showTasks:   true,

		operationMode:      setting.ModeNormal,
		sessionPermissions: setting.NewSessionPermissions(),
		disabledTools:      setting.GetDisabledTools(),

		llmProvider:   llm.DefaultSetup.Provider,
		providerStore: llm.DefaultSetup.Store,
		currentModel:  llm.DefaultSetup.CurrentModel,

		sessionStore: session.DefaultSetup.Store,
		sessionID:    session.DefaultSetup.SessionID,

		provider: newProviderState(),
		session:  newSessionState(),
		memory:   newMemoryState(),
		mode:     newModeState(),
		tool:     newToolState(),
		mcp:      newMCPState(),
		plugin:   newPluginState(),
		approval: appapproval.New(),
		isGit:    setting.IsGitRepo(appCwd),

		systemInput: appsystem.New(),
		settings:    setting.DefaultSetup,
		hookEngine:  hook.DefaultEngine,
		fileWatcher: appsystem.NewFileWatcher(hook.DefaultEngine, nil),
		agentInput:  appagent.New(),
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
	// Plugin already loaded via LoadFromPath — only refresh dependent registries
	skill.Initialize(m.cwd)
	command.SetDynamicInfoProviders(skillCommandInfos)
	command.Initialize(m.cwd)
	subagent.Initialize(m.cwd)
	mcp.Initialize(m.cwd)

	setting.Initialize(m.cwd)
	m.settings = setting.DefaultSetup
	if m.hookEngine != nil {
		m.hookEngine.SetSettings(setting.DefaultSetup)
	}
	m.reconfigureAgentTool()

	return nil
}

func (m *model) enablePlanMode(prompt string) error {
	m.planEnabled = true
	m.planTask = prompt
	m.operationMode = setting.ModePlan

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
	if err := subagent.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize subagent", zap.Error(err))
	}
	if err := mcp.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize mcp", zap.Error(err))
	}
}


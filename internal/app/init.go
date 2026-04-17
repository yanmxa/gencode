package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/app/conv"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/app/input"

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

	// 6. Hook engine — assemble dependencies for the hook package
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
	// plugin env vars (e.g., GEN_PLUGIN_<name>_ROOT) injected into Bash child processes
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
		tool:        newToolState(),
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

func newToolState() conv.ToolState {
	return conv.ToolState{Selector: conv.NewToolSelector(
		setting.GetDisabledToolsAt,
		setting.UpdateDisabledToolsAt,
	)}
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

// pluginMCPServers adapts plugin.GetPluginMCPServers() to the mcp.PluginServer
// type so that the mcp package doesn't import plugin directly.
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

// agentRegistryAdapter adapts *subagent.Registry to the input.AgentRegistry
// interface so app/user doesn't import subagent directly.
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


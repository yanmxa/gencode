package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appcommand "github.com/yanmxa/gencode/internal/app/command"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appinput "github.com/yanmxa/gencode/internal/app/input"
	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	appskill "github.com/yanmxa/gencode/internal/app/skill"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/progress"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

type modelInfra struct {
	store             *provider.Store
	llmProvider       provider.LLMProvider
	currentModel      *provider.CurrentModelInfo
	mcpRegistry       *mcp.Registry
	settings          *config.Settings
	hookEngine        *hooks.Engine
	earlySessionStore *session.Store
}

func initializeModelInfra(cwd string) (modelInfra, error) {
	store, llmProvider, currentModel := initializeProvider()
	mcpRegistry := initializeRegistries(cwd)
	settings := loadSettings()

	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())

	var transcriptPath string
	earlySessionStore, _ := session.NewStore(cwd)
	if earlySessionStore != nil {
		transcriptPath = earlySessionStore.SessionPath(sessionID)
	}
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, transcriptPath)

	return modelInfra{
		store:             store,
		llmProvider:       llmProvider,
		currentModel:      currentModel,
		mcpRegistry:       mcpRegistry,
		settings:          settings,
		hookEngine:        hookEngine,
		earlySessionStore: earlySessionStore,
	}, nil
}

func newBaseModel(cwd string, infra modelInfra) model {
	matchFunc := func(query string) []suggest.Suggestion {
		cmds := appcommand.GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	}

	progressHub := progress.NewHub(100)

	return model{
		input:  appinput.New(cwd, defaultWidth, matchFunc),
		output: appoutput.New(defaultWidth, progressHub),
		conv:   appconv.New(),
		cwd:    cwd,

		provider: appprovider.State{
			LLM:          infra.llmProvider,
			Store:        infra.store,
			CurrentModel: infra.currentModel,
			Selector:     appprovider.New(),
		},
		session: appsession.State{
			Selector: appsession.New(),
			Store:    infra.earlySessionStore,
		},
		skill: appskill.State{
			Selector: appskill.New(),
		},
		memory: appmemory.State{
			Selector: appmemory.New(),
		},
		mode: appmode.State{
			Operation:          appmode.Normal,
			SessionPermissions: config.NewSessionPermissions(),
			DisabledTools:      config.GetDisabledTools(),
			PlanApproval:       appmode.NewPlanPrompt(),
			PlanEntry:          appmode.NewEnterPlanPrompt(),
			Question:           appmode.NewQuestionPrompt(),
		},
		tool:     apptool.State{Selector: apptool.New()},
		mcp:      appmcp.State{Selector: appmcp.New(), Registry: infra.mcpRegistry},
		plugin:   appplugin.State{Selector: appplugin.New()},
		agent:    appagent.State{Selector: appagent.New()},
		approval: appapproval.New(),

		showTasks: true,
		isGit:     isGitRepo(cwd),

		settings:   infra.settings,
		hookEngine: infra.hookEngine,
		loop:       &core.Loop{},
		runtime:    newConversationRuntime(),
	}
}

func (m *model) applyRunOptions(opts options.RunOptions) error {
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := plugin.DefaultRegistry.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
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

func (m *model) enablePlanMode(prompt string) error {
	m.mode.Enabled = true
	m.mode.Task = prompt
	m.mode.Operation = appmode.Plan

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
		forked, err := sessionStore.Fork(sess.Metadata.ID, sess)
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
			forked, err := sessionStore.Fork(sess.Metadata.ID, sess)
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
			tool.DefaultTodoStore.SetStorageDir(filepath.Join(homeDir, ".gen", "tasks", taskListID))
		}
	}
}

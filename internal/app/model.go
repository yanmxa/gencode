// Core data types, message commit pipeline, and LLM loop configuration.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appcommand "github.com/yanmxa/gencode/internal/app/command"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appinput "github.com/yanmxa/gencode/internal/app/input"
	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	appskill "github.com/yanmxa/gencode/internal/app/skill"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

const defaultWidth = 80

type model struct {
	// IO
	input  appinput.Model
	output appoutput.Model

	// Terminal
	width  int
	height int
	ready  bool
	cwd    string

	// Conversation
	conv appconv.Model

	// Domain — each feature owns all its state
	provider appprovider.State
	session  appsession.State
	skill    appskill.State
	memory   appmemory.State
	mode     appmode.State
	tool     apptool.State
	mcp      appmcp.State
	plugin   appplugin.State
	agent    appagent.State
	approval *appapproval.Model

	// Config and Infra
	settings   *config.Settings
	hookEngine *hooks.Engine
	loop       *core.Loop
}

// --- Constructor and Init ---
func newModel(opts options.RunOptions) (model, error) {
	cwd, _ := os.Getwd()

	// Initialize components
	store, llmProvider, currentModel := initializeProvider()
	mcpRegistry := initializeRegistries(cwd)
	settings := loadSettings()

	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, "")

	if llmProvider != nil {
		modelID := ""
		if currentModel != nil {
			modelID = currentModel.ModelID
		}
		appprovider.ConfigureTaskTool(llmProvider, cwd, modelID, hookEngine, nil, "")
	}

	matchFunc := func(query string) []suggest.Suggestion {
		cmds := appcommand.GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	}

	m := model{
		input:  appinput.New(cwd, defaultWidth, matchFunc),
		output: appoutput.New(defaultWidth),
		conv:   appconv.New(),
		cwd:    cwd,

		provider: appprovider.State{
			LLM:          llmProvider,
			Store:        store,
			CurrentModel: currentModel,
			Selector:     appprovider.New(),
		},
		session: appsession.State{
			Selector: appsession.New(),
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
		tool:   apptool.State{Selector: apptool.New()},
		mcp:    appmcp.State{Selector: appmcp.New(), Registry: mcpRegistry},
		plugin: appplugin.State{Selector: appplugin.New()},
		agent:    appagent.State{Selector: appagent.New()},
		approval: appapproval.New(),

		settings:   settings,
		hookEngine: hookEngine,
		loop:       &core.Loop{},
	}

	// Apply run options
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := plugin.DefaultRegistry.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return model{}, fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
		}
	}

	if opts.PlanMode {
		m.mode.Enabled = true
		m.mode.Task = opts.Prompt
		m.mode.Operation = appmode.Plan

		planStore, err := plan.NewStore()
		if err != nil {
			return model{}, fmt.Errorf("failed to initialize plan store: %w", err)
		}
		m.mode.Store = planStore
	}

	if opts.Continue {
		sessionStore, err := session.NewStore(cwd)
		if err != nil {
			return model{}, fmt.Errorf("failed to initialize session store: %w", err)
		}
		m.session.Store = sessionStore

		sess, err := sessionStore.GetLatest()
		if err != nil {
			return model{}, fmt.Errorf("no previous session to continue: %w", err)
		}

		m.restoreSessionData(sess)
	}

	if opts.Resume {
		sessionStore, err := session.NewStore(cwd)
		if err != nil {
			return model{}, fmt.Errorf("failed to initialize session store: %w", err)
		}
		m.session.Store = sessionStore
		m.session.PendingSelector = true
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	if m.hookEngine != nil {
		source := "startup"
		if m.session.CurrentID != "" {
			source = "resume"
		}
		m.hookEngine.ExecuteAsync(hooks.SessionStart, hooks.HookInput{
			Source: source,
			Model:  m.getModelID(),
		})
	}
	return tea.Batch(textarea.Blink, m.output.Spinner.Tick, appmcp.AutoConnect())
}

// --- Message commit pipeline ---

func (m *model) commitMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(true)
}

func (m *model) commitAllMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(false)
}

func (m *model) commitMessagesWithCheck(checkReady bool) []tea.Cmd {
	var cmds []tea.Cmd
	lastIdx := len(m.conv.Messages) - 1

	for i := m.conv.CommittedCount; i < len(m.conv.Messages); i++ {
		msg := m.conv.Messages[i]

		if checkReady {
			if i == lastIdx && msg.Role == message.RoleAssistant && m.conv.Stream.Active {
				break
			}
			if msg.Role == message.RoleAssistant && len(msg.ToolCalls) > 0 && !m.conv.HasAllToolResults(i) {
				break
			}
		}

		if rendered := m.renderSingleMessage(i); rendered != "" {
			cmds = append(cmds, tea.Println(rendered))
		}
		m.conv.CommittedCount = i + 1
	}
	return cmds
}

// --- Message conversion and LLM loop configuration ---

func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

func (m *model) configureLoop(extra []string) {
	var mcpToolsGetter func() []provider.Tool
	if m.mcp.Registry != nil {
		mcpToolsGetter = m.mcp.Registry.GetToolSchemas
	}

	if m.memory.CachedUser == "" && m.memory.CachedProject == "" {
		m.memory.CachedUser, m.memory.CachedProject = system.LoadInstructions(m.cwd)
	}

	var skills, agents string
	if skill.DefaultRegistry != nil {
		skills = skill.DefaultRegistry.GetSkillsSection()
	}
	if agent.DefaultRegistry != nil {
		agents = agent.DefaultRegistry.GetAgentsSection()
	}
	var sessionSummary string
	if m.session.Summary != "" {
		sessionSummary = fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.session.Summary)
	}

	m.loop.Client = &client.Client{
		Provider:  m.provider.LLM,
		Model:     m.getModelID(),
		MaxTokens: m.getMaxTokens(),
	}
	m.loop.System = &system.System{
		Client:              m.loop.Client,
		Cwd:                 m.cwd,
		IsGit:               isGitRepo(m.cwd),
		PlanMode:            m.mode.Enabled,
		UserInstructions:    m.memory.CachedUser,
		ProjectInstructions: m.memory.CachedProject,
		SessionSummary:      sessionSummary,
		Skills:              skills,
		Agents:              agents,
		Extra:               extra,
	}
	m.loop.Tool = &tool.Set{
		Disabled: m.mode.DisabledTools,
		PlanMode: m.mode.Enabled,
		MCP:      mcpToolsGetter,
	}
	m.loop.Permission = nil
	m.loop.Hooks = m.hookEngine
}

func (m model) getModelID() string {
	if m.provider.CurrentModel != nil {
		return m.provider.CurrentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

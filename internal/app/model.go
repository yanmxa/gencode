// Core data types, message commit pipeline, and LLM loop configuration.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"strings"

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

const (
	defaultWidth = 80

	// taskReminderThreshold is the number of LLM turns without any Task* tool use
	// before a reminder is injected into the system prompt.
	taskReminderThreshold = 5
)

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

	// UI toggles
	showTasks    bool   // Ctrl+T toggles task list visibility
	isGit        bool   // cached: whether cwd is a git repository
	initialPrompt string // initial prompt from CLI args

	// Config and Infra
	settings         *config.Settings
	hookEngine       *hooks.Engine
	loop             *core.Loop
	promptSuggestion promptSuggestionState
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
		userInstr, projInstr := system.LoadInstructions(cwd)
		appprovider.ConfigureAgentTool(llmProvider, cwd, modelID, hookEngine, nil, "",
			appprovider.WithContext(userInstr, projInstr, isGitRepo(cwd)))
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

		showTasks: true,
		isGit:     isGitRepo(cwd),

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

	if opts.Prompt != "" && !opts.PlanMode {
		m.initialPrompt = opts.Prompt
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

		if opts.Fork {
			forked, err := sessionStore.Fork(sess.Metadata.ID, sess)
			if err != nil {
				return model{}, fmt.Errorf("failed to fork session: %w", err)
			}
			sess = forked
		}

		m.restoreSessionData(sess)
	}

	if opts.Resume {
		sessionStore, err := session.NewStore(cwd)
		if err != nil {
			return model{}, fmt.Errorf("failed to initialize session store: %w", err)
		}
		m.session.Store = sessionStore

		if opts.ResumeID != "" {
			sess, err := sessionStore.Load(opts.ResumeID)
			if err != nil {
				return model{}, fmt.Errorf("failed to load session %s: %w", opts.ResumeID, err)
			}
			if opts.Fork {
				forked, err := sessionStore.Fork(sess.Metadata.ID, sess)
				if err != nil {
					return model{}, fmt.Errorf("failed to fork session: %w", err)
				}
				sess = forked
			}
			m.restoreSessionData(sess)
		} else {
			// No ID — open session selector
			m.session.PendingSelector = true
			m.session.PendingFork = opts.Fork
		}
	}

	// When GEN_TASK_LIST_ID is set, initialize task storage immediately
	// (don't wait for session creation) so tasks persist from the start.
	if taskListID := os.Getenv("GEN_TASK_LIST_ID"); taskListID != "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			tool.DefaultTodoStore.SetStorageDir(filepath.Join(homeDir, ".gen", "tasks", taskListID))
		}
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

	cmds := []tea.Cmd{textarea.Blink, m.output.Spinner.Tick, appmcp.AutoConnect()}
	if m.initialPrompt != "" {
		prompt := m.initialPrompt
		m.initialPrompt = ""
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
			if i == lastIdx && msg.Role == message.RoleAssistant && m.conv.Stream.Active {
				break
			}
			if msg.Role == message.RoleAssistant && len(msg.ToolCalls) > 0 && !m.conv.HasAllToolResults(i) {
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

func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

// reconfigureAgentTool updates the agent tool with the current session/provider state.
func (m *model) reconfigureAgentTool() {
	if m.provider.LLM != nil {
		appprovider.ConfigureAgentTool(m.provider.LLM, m.cwd, m.getModelID(), m.hookEngine, m.session.Store, m.session.CurrentID,
			m.agentToolOpts()...)
	}
}

// agentToolOpts returns the common options for ConfigureAgentTool calls.
func (m *model) agentToolOpts() []appprovider.AgentToolOption {
	opts := []appprovider.AgentToolOption{
		appprovider.WithContext(m.memory.CachedUser, m.memory.CachedProject, m.isGit),
	}
	if m.mcp.Registry != nil {
		opts = append(opts, appprovider.WithMCP(m.mcp.Registry.GetToolSchemas, m.mcp.Registry))
	}
	return opts
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

	// Include active skill invocation if present
	allExtra := extra
	if m.skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.skill.ActiveInvocation)
	}

	// Inject task reminder nudge when tasks exist and haven't been updated recently
	if reminder := m.buildTaskReminder(); reminder != "" {
		allExtra = append(allExtra, reminder)
	}

	m.loop.Client = &client.Client{
		Provider:      m.provider.LLM,
		Model:         m.getModelID(),
		MaxTokens:     m.getMaxTokens(),
		ThinkingLevel: m.effectiveThinkingLevel(),
	}
	m.loop.System = &system.System{
		Client:              m.loop.Client,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.mode.Enabled,
		UserInstructions:    m.memory.CachedUser,
		ProjectInstructions: m.memory.CachedProject,
		SessionSummary:      sessionSummary,
		Skills:              skills,
		Agents:              agents,
		Extra:               allExtra,
	}
	m.loop.Tool = &tool.Set{
		Disabled: m.mode.DisabledTools,
		PlanMode: m.mode.Enabled,
		MCP:      mcpToolsGetter,
	}
	m.loop.Permission = nil
	m.loop.Hooks = m.hookEngine
}

// effectiveThinkingLevel returns the higher of the persistent level and the per-turn override.
func (m *model) effectiveThinkingLevel() provider.ThinkingLevel {
	return max(m.provider.ThinkingLevel, m.provider.ThinkingOverride)
}

// buildTaskReminder returns a task reminder string if tasks exist and haven't
// been updated for taskReminderThreshold turns. Returns empty string otherwise.
func (m *model) buildTaskReminder() string {
	if m.conv.TurnsSinceLastTaskTool < taskReminderThreshold {
		return ""
	}
	tasks := tool.DefaultTodoStore.List()
	if len(tasks) == 0 {
		return ""
	}

	// Check if all tasks are completed
	allDone := true
	for _, t := range tasks {
		if t.Status != tool.TodoStatusCompleted {
			allDone = false
			break
		}
	}
	if allDone {
		return ""
	}

	// Build reminder with current task list
	var sb strings.Builder
	sb.WriteString("<task-reminder>\n")
	sb.WriteString("You have active tasks that haven't been updated recently. Consider updating task status:\n")
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("  %s #%s: %s [%s]\n", tool.TaskIcon(t), t.ID, t.Subject, t.Status))
	}
	sb.WriteString("Use TaskUpdate to mark tasks as in_progress when starting or completed when done.\n")
	sb.WriteString("</task-reminder>")
	return sb.String()
}

func (m model) getModelID() string {
	if m.provider.CurrentModel != nil {
		return m.provider.CurrentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

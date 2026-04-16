// Core data types, message commit pipeline, agent session management, and LLM loop configuration.
//
// Agent builder (buildCoreAgent, ensureAgentSession, startAgentLoop) lives here
// because it is Model initialization, not an Update handler.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/app/user/agentui"
	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	appmode "github.com/yanmxa/gencode/internal/app/user/mode"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	appqueue "github.com/yanmxa/gencode/internal/app/user/queue"
	"github.com/yanmxa/gencode/internal/app/user/searchui"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/app/user/skillui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/tasktools"
	"github.com/yanmxa/gencode/internal/util/filecache"
)

const (
	defaultWidth = 80

	// taskReminderThreshold is the number of LLM turns without any Task* tool use
	// before a reminder is injected into the system prompt.
	taskReminderThreshold = 5
)

type model struct {
	// Source 1: userInput — textarea, history, images, queue, overlay, modal, mode
	userInput        appuser.Model
	inputQueue       appqueue.Queue
	mode             appmode.State
	approval         *appapproval.Model
	promptSuggestion promptSuggestionState

	// Source 1 overlays — each selector is user-triggered
	provider providerui.State
	session  sessionui.State
	skill    skillui.State
	memory   appmemory.State
	tool     toolui.State
	mcp      mcpui.State
	plugin   pluginui.State
	agent    agentui.State
	search   searchui.State

	// Source 2: agentInput — background agent notifications, batch tracking
	agentInput appagent.State

	// Source 3: systemInput — cron scheduler and async hook rewakes
	systemInput appsystem.State

	// Agent Output — conversation, stream, tokens, provider, session, compact
	conv        appconv.Model
	agentOutput appoutput.Model

	// Config — settings, hookEngine, fileCache, cwd, isGit
	width         int
	height        int
	ready         bool
	cwd           string
	isGit         bool
	initialPrompt string
	settings      *config.Settings
	hookEngine    *hooks.Engine
	fileWatcher   *appsystem.FileWatcher
	fileCache     *filecache.Cache

	// Agent session
	agentSess         *agentSession
	pendingPermBridge *appoutput.PermBridgeRequest
}

// --- Constructor and Init ---
func initModel(opts config.RunOptions) (*model, error) {
	cwd, _ := os.Getwd()
	infra, err := initInfra(cwd)
	if err != nil {
		return nil, err
	}
	base := newBaseModel(cwd, infra)
	m := &base
	if m.hookEngine != nil && m.systemInput.AsyncHookQueue != nil {
		queue := m.systemInput.AsyncHookQueue
		m.hookEngine.SetAsyncHookCallback(func(result hooks.AsyncHookResult) {
			reason := result.BlockReason
			if reason == "" {
				reason = "asynchronous hook requested a rewake"
			}
			queue.Push(appsystem.AsyncHookRewake{
				Notice:             fmt.Sprintf("Async hook blocked: %s", reason),
				Context:            []string{formatAsyncHookContinuationContext(result, reason)},
				ContinuationPrompt: "A background policy hook reported a blocking condition. Re-evaluate the plan and choose a safer next step.",
			})
		})
	}

	m.ensureMemoryContextLoaded()
	m.reconfigureAgentTool()
	if err := m.applyRunOptions(opts); err != nil {
		return nil, err
	}
	m.initTaskStorage()

	// Fire SessionStart during construction so hook-driven mutations apply
	// before Bubble Tea starts driving the pointer-backed model.
	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Setup, hooks.HookInput{
			Trigger: "init",
		})
		source := "startup"
		if m.session.CurrentID != "" {
			source = "resume"
		}
		outcome := m.hookEngine.Execute(context.Background(), hooks.SessionStart, hooks.HookInput{
			Source: source,
			Model:  m.getModelID(),
		})
		m.applyRuntimeHookOutcome(outcome)
		if outcome.AdditionalContext != "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleUser,
				Content: outcome.AdditionalContext,
			})
		}
	}

	return m, nil
}

// lastAssistantContent returns the text content of the most recent assistant core.
func (m *model) lastAssistantContent() string {
	return core.LastAssistantChatContent(m.conv.Messages)
}

// fireSessionEnd fires the SessionEnd hook synchronously before quitting.
// Uses Execute (not ExecuteAsync) to ensure the hook completes before the process exits.
func (m *model) fireSessionEnd(reason string) {
	if m.hookEngine != nil {
		m.hookEngine.Execute(context.Background(), hooks.SessionEnd, hooks.HookInput{
			Reason: reason,
		})
		if m.fileWatcher != nil {
			m.fileWatcher.Stop()
		}
		m.hookEngine.ClearSessionHooks()
	}
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.agentOutput.Spinner.Tick, mcpui.AutoConnect(), appsystem.TriggerCronTickNow(), appsystem.StartCronTicker(), appsystem.StartAsyncHookTicker(), appagent.StartTicker()}
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
	if m.provider.LLM != nil {
		m.ensureMemoryContextLoaded()
		configureAgentTool(m.provider.LLM, m.cwd, m.getModelID(), m.hookEngine, m.session.Store, m.session.CurrentID,
			m.agentToolOpts()...)
	}
}

func (m *model) agentToolOpts() []agentToolOption {
	opts := []agentToolOption{
		withAgentContext(m.memory.CachedUser, m.memory.CachedProject, m.isGit),
	}
	if m.mcp.Registry != nil {
		opts = append(opts, withAgentMCP(m.mcp.Registry.GetToolSchemas, m.mcp.Registry))
	}
	return opts
}

func (m *model) ensureMemoryContextLoaded() {
	if m.memory.CachedUser != "" || m.memory.CachedProject != "" {
		return
	}
	m.refreshMemoryContext("session_start")
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
	tasks := tracker.DefaultStore.List()
	if len(tasks) == 0 {
		return ""
	}

	// Check if all tasks are completed
	allDone := true
	for _, t := range tasks {
		if t.Status != tracker.StatusCompleted {
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
		sb.WriteString(fmt.Sprintf("  %s #%s: %s [%s]\n", tasktools.TaskIcon(t), t.ID, t.Subject, t.Status))
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

func formatAsyncHookContinuationContext(result hooks.AsyncHookResult, reason string) string {
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
	agent      core.Agent
	permBridge *appoutput.PermissionBridge
	cancel     context.CancelFunc
}

// buildCoreAgent creates a core.Agent and permissionBridge from the model's
// current state. The agent is not started — call startAgentLoop() for that.
func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.provider.LLM == nil {
		return nil, errNoProvider
	}

	// LLM — wraps the current provider as core.LLM
	client := provider.NewClient(m.provider.LLM, m.getModelID(), m.getMaxTokens())
	client.SetThinking(m.effectiveThinkingLevel())

	// System prompt — build layered core.System directly
	c := m.buildLoopClient()
	sys := m.buildLoopSystem(nil, c)

	// Tools — adapt legacy tool registry to core.Tools
	toolSchemas := m.buildLoopToolSet().Tools()
	coreSchemas := make([]core.ToolSchema, len(toolSchemas))
	for i, ts := range toolSchemas {
		coreSchemas[i] = core.ToolSchema{
			Name:        ts.Name,
			Description: ts.Description,
			Parameters:  ts.Parameters,
		}
	}
	tools := tool.AdaptToolRegistry(coreSchemas, func() string { return m.cwd })

	// Hooks — wrap hooks.Engine as core.Hooks
	coreHooks := hooks.AsCoreHooks(m.hookEngine)

	// Permission bridge — blocking PermissionFunc with TUI approval
	permBridge := appoutput.NewPermissionBridge(
		func() *config.Settings { return m.settings },
		func() *config.SessionPermissions { return m.mode.SessionPermissions },
		func() string { return m.cwd },
	)

	ag := core.NewAgent(core.Config{
		ID:         "main",
		LLM:        client,
		System:     sys,
		Tools:      tools,
		Hooks:      coreHooks,
		Permission: permBridge.PermissionFunc(),
		CWD:        m.cwd,
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
		appoutput.DrainAgentOutbox(sess.agent.Outbox()),
		pollPermBridge(sess.permBridge),
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

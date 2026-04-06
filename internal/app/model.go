// Core data types, message commit pipeline, and LLM loop configuration.
package app

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"strings"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
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
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
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

	// Cron scheduler
	cronQueue []string // queued cron prompts waiting for idle REPL

	// Max-output-tokens recovery counter (reset on new user input)
	maxOutputRecoveryCount int

	// UI toggles
	showTasks     bool   // Ctrl+T toggles task list visibility
	isGit         bool   // cached: whether cwd is a git repository
	initialPrompt string // initial prompt from CLI args
	hookStatus    string // temporary active hook status shown in status bar

	// Config and Infra
	settings         *config.Settings
	hookEngine       *hooks.Engine
	fileWatcher      *fileWatcher
	asyncHookQueue   *asyncHookQueue
	loop             *core.Loop
	runtime          conversationRuntime
	promptSuggestion promptSuggestionState
}

// --- Constructor and Init ---
func newModel(opts options.RunOptions) (model, error) {
	cwd, _ := os.Getwd()
	infra, err := initializeModelInfra(cwd)
	if err != nil {
		return model{}, err
	}
	m := newBaseModel(cwd, infra)
	if m.hookEngine != nil && m.asyncHookQueue != nil {
		queue := m.asyncHookQueue
		m.hookEngine.SetAsyncHookCallback(func(result hooks.AsyncHookResult) {
			reason := result.BlockReason
			if reason == "" {
				reason = "asynchronous hook requested a rewake"
			}
			queue.Push(asyncHookRewake{
				Notice:             fmt.Sprintf("Async hook blocked: %s", reason),
				Context:            []string{formatAsyncHookContinuationContext(result, reason)},
				ContinuationPrompt: "A background policy hook reported a blocking condition. Re-evaluate the plan and choose a safer next step.",
			})
		})
	}

	m.ensureMemoryContextLoaded()
	m.reconfigureAgentTool()
	if err := m.applyRunOptions(opts); err != nil {
		return model{}, err
	}
	m.initializeTaskStorageFromEnv()

	return m, nil
}

// lastAssistantContent returns the text content of the most recent assistant message.
func (m *model) lastAssistantContent() string {
	return message.LastAssistantChatContent(m.conv.Messages)
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

func (m model) Init() tea.Cmd {
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
			m.conv.Append(message.ChatMessage{
				Role:    message.RoleUser,
				Content: outcome.AdditionalContext,
			})
		}
	}

	cmds := []tea.Cmd{textarea.Blink, m.output.Spinner.Tick, appmcp.AutoConnect(), triggerCronTickNow(), startCronTicker(), startAsyncHookTicker()}
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
		m.ensureMemoryContextLoaded()
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
	m.ensureMemoryContextLoaded()
	m.loop.Client = m.buildLoopClient()
	m.loop.System = m.buildLoopSystem(extra, m.loop.Client)
	m.loop.Tool = m.buildLoopToolSet()
	m.loop.Permission = nil
	m.loop.Hooks = m.hookEngine
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

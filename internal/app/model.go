// Core data types, message commit pipeline, and LLM loop configuration.
package app

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"strings"

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
	appqueue "github.com/yanmxa/gencode/internal/app/queue"
	"github.com/yanmxa/gencode/internal/app/sessionui"
	"github.com/yanmxa/gencode/internal/app/skillui"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/tool/tasktools"
	"github.com/yanmxa/gencode/internal/tracker"
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
	provider providerui.State
	session  sessionui.State
	skill    skillui.State
	memory   appmemory.State
	mode     appmode.State
	tool     toolui.State
	mcp      mcpui.State
	plugin   pluginui.State
	agent    agentui.State
	approval *appapproval.Model

	// Input queue — buffers user messages submitted while the LLM is busy
	inputQueue     appqueue.Queue
	queueSelectIdx int    // -1 = no selection, 0+ = selected queue item index
	queueTempInput string // stashed input when navigating into queue

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
	settings          *config.Settings
	hookEngine        *hooks.Engine
	fileWatcher       *fileWatcher
	asyncHookQueue    *asyncHookQueue
	taskNotifications *taskNotificationQueue
	loop              *runtime.Loop
	runtime           conversationRuntime
	promptSuggestion  promptSuggestionState
	fileCache         *filecache.Cache
}

// --- Constructor and Init ---
func newModel(opts config.RunOptions) (model, error) {
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
	m.initTaskStorage()

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

	cmds := []tea.Cmd{textarea.Blink, m.output.Spinner.Tick, mcpui.AutoConnect(), triggerCronTickNow(), startCronTicker(), startAsyncHookTicker(), startTaskNotificationTicker()}
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

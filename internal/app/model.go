// Core data types, message commit pipeline, agent session management, and LLM loop configuration.
//
// Agent builder (buildCoreAgent, ensureAgentSession, startAgentLoop) lives here
// because it is Model initialization, not an Update handler.
package app

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/app/conv"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

const defaultWidth = 80

type model struct {
	// ── User Input ──────────────────────────────────────────────────────
	userInput        input.Model
	mode             conv.ModalState
	promptSuggestion promptSuggestionState
	showTasks        bool
	tool             conv.ToolState

	// ── Agent Input ─────────────────────────────────────────────────────
	agentInput notify.Model

	// ── System Input ────────────────────────────────────────────────────
	systemInput trigger.Model

	// ── Agent Output ────────────────────────────────────────────────────
	conv                 conv.ConversationModel
	agentOutput          conv.Model
	agentSess            *agentSession
	pendingQuestion      *tool.QuestionRequest
	pendingQuestionReply chan *tool.QuestionResponse

	// ── Runtime (shared state: provider, session, permission, plan, config) ──
	runtime appruntime.Model

	// ── Infrastructure ──────────────────────────────────────────────
	cwd           string
	isGit         bool
	width         int
	height        int
	ready         bool
	initialPrompt string
	fileWatcher   *trigger.FileWatcher
	fileCache     *filecache.Cache
}

func (m *model) fireSessionEnd(reason string) {
	m.runtime.FireSessionEnd(context.Background(), reason)
	if m.fileWatcher != nil {
		m.fileWatcher.Stop()
	}
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.agentOutput.Spinner.Tick, m.userInput.MCP.Selector.AutoConnect(), trigger.TriggerCronTickNow(), trigger.StartCronTicker(), trigger.StartAsyncHookTicker(), notify.StartTicker()}
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
	if m.runtime.LLMProvider != nil {
		m.ensureMemoryContextLoaded()
		configureAgentTool(m.runtime.LLMProvider, m.cwd, m.getModelID(), m.runtime.HookEngine, m.runtime.SessionStore, m.runtime.SessionID,
			m.agentToolOpts()...)
	}
}

func (m *model) agentToolOpts() []agentToolOption {
	opts := []agentToolOption{
		withAgentContext(m.runtime.CachedUserInstructions, m.runtime.CachedProjectInstructions, m.isGit),
	}
	if mcp.DefaultRegistry != nil {
		opts = append(opts, withAgentMCP(mcp.DefaultRegistry.GetToolSchemas, mcp.DefaultRegistry))
	}
	return opts
}

func (m *model) ensureMemoryContextLoaded() {
	if m.runtime.CachedUserInstructions != "" || m.runtime.CachedProjectInstructions != "" {
		return
	}
	m.runtime.RefreshMemoryContext(m.cwd, "session_start")
}

func (m *model) effectiveThinkingLevel() llm.ThinkingLevel {
	return m.runtime.EffectiveThinkingLevel()
}

func (m model) getModelID() string {
	return m.runtime.GetModelID()
}

func formatAsyncHookContinuationContext(result hook.AsyncHookResult, reason string) string {
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
	agent              core.Agent
	permBridge         *conv.PermissionBridge
	cancel             context.CancelFunc
	pendingPermRequest *conv.PermBridgeRequest
}

// buildCoreAgent creates a core.Agent and permissionBridge from the model's
// current state. The agent is not started — call startAgentLoop() for that.
func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.runtime.LLMProvider == nil {
		return nil, errNoProvider
	}

	// LLM — wraps the current provider as core.LLM
	client := llm.NewClient(m.runtime.LLMProvider, m.getModelID(), m.getMaxTokens())
	client.SetThinking(m.effectiveThinkingLevel())

	// System prompt — build layered core.System directly
	c := m.buildLoopClient()
	sys := m.buildLoopSystem(nil, c)

	// Tools — adapt legacy tool registry to core.Tools
	schemas := m.buildLoopToolSet().Tools()
	tools := tool.AdaptToolRegistry(schemas, func() string { return m.cwd })

	// MCP tools — add MCP tool executors so core.Agent can execute them
	if mcp.DefaultRegistry != nil {
		mcpCaller := mcp.NewCaller(mcp.DefaultRegistry)
		for _, t := range mcp.AsCoreTools(schemas, mcpCaller) {
			tools.Add(t)
		}
	}

	// Permission bridge — blocking PermissionFunc with TUI approval
	permBridge := conv.NewPermissionBridge(func(name string, args map[string]any) conv.PermDecisionResult {
		settings := m.runtime.Settings
		if settings == nil {
			return conv.PermDecisionResult{Decision: perm.Permit}
		}
		decision := settings.HasPermissionToUseTool(name, args, m.runtime.SessionPermissions)
		switch decision.Behavior {
		case setting.Allow:
			return conv.PermDecisionResult{Decision: perm.Permit, Reason: decision.Reason}
		case setting.Deny:
			return conv.PermDecisionResult{Decision: perm.Reject, Reason: decision.Reason}
		default:
			return conv.PermDecisionResult{
				Decision:    perm.Prompt,
				Reason:      decision.Reason,
				ToolName:    name,
				Description: decision.Reason,
			}
		}
	})

	// Wrap tools with permission decorator
	permTools := tool.WithPermission(tools, permBridge.PermissionFunc())

	ag := core.NewAgent(core.Config{
		ID:     "main",
		LLM:    client,
		System: sys,
		Tools:  permTools,
		CWD:    m.cwd,
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
		conv.DrainAgentOutbox(sess.agent.Outbox()),
		conv.PollPermBridge(sess.permBridge),
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

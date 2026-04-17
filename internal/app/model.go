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

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	appmodal "github.com/yanmxa/gencode/internal/app/output/modal"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/filecache"
)

const defaultWidth = 80

type model struct {
	// ── User Input ──────────────────────────────────────────────────────
	userInput        appuser.Model
	approval         appapproval.Model
	mode             appmodal.State
	promptSuggestion promptSuggestionState
	showTasks        bool
	provider         providerui.State
	session          sessionui.State
	memory           appmemory.State
	tool             toolui.State
	mcp              mcpui.State
	plugin           pluginui.Model

	// ── Agent Input ─────────────────────────────────────────────────────
	agentInput appagent.State

	// ── System Input ────────────────────────────────────────────────────
	systemInput appsystem.State

	// ── Agent Output ────────────────────────────────────────────────────
	conv                 appconv.Model
	agentOutput          appoutput.Model
	agentSess            *agentSession
	pendingQuestion      *tool.QuestionRequest
	pendingQuestionReply chan *tool.QuestionResponse

	// ── Runtime ─────────────────────────────────────────────────────────
	cwd           string
	isGit         bool
	width         int
	height        int
	ready         bool
	initialPrompt string

	operationMode      setting.OperationMode
	sessionPermissions *setting.SessionPermissions
	disabledTools      map[string]bool

	llmProvider      llm.Provider
	providerStore    *llm.Store
	currentModel     *llm.CurrentModelInfo
	inputTokens      int
	outputTokens     int
	thinkingLevel    llm.ThinkingLevel
	thinkingOverride llm.ThinkingLevel

	sessionStore   *session.Store
	sessionID      string
	sessionSummary string

	planEnabled bool
	planTask    string
	planStore   *plan.Store

	cachedUserInstructions    string
	cachedProjectInstructions string

	settings    *setting.Settings
	hookEngine  *hook.Engine
	fileWatcher *appsystem.FileWatcher
	fileCache   *filecache.Cache
}

// lastAssistantContent returns the text content of the most recent assistant core.
func (m *model) lastAssistantContent() string {
	return core.LastAssistantChatContent(m.conv.Messages)
}

// fireSessionEnd fires the SessionEnd hook synchronously before quitting.
// Uses Execute (not ExecuteAsync) to ensure the hook completes before the process exits.
func (m *model) fireSessionEnd(reason string) {
	if m.hookEngine != nil {
		m.hookEngine.Execute(context.Background(), hook.SessionEnd, hook.HookInput{
			Reason: reason,
		})
		if m.fileWatcher != nil {
			m.fileWatcher.Stop()
		}
		m.hookEngine.ClearSessionHooks()
	}
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, m.agentOutput.Spinner.Tick, m.mcp.Selector.AutoConnect(), appsystem.TriggerCronTickNow(), appsystem.StartCronTicker(), appsystem.StartAsyncHookTicker(), appagent.StartTicker()}
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
	if m.llmProvider != nil {
		m.ensureMemoryContextLoaded()
		configureAgentTool(m.llmProvider, m.cwd, m.getModelID(), m.hookEngine, m.sessionStore, m.sessionID,
			m.agentToolOpts()...)
	}
}

func (m *model) agentToolOpts() []agentToolOption {
	opts := []agentToolOption{
		withAgentContext(m.cachedUserInstructions, m.cachedProjectInstructions, m.isGit),
	}
	if mcp.DefaultRegistry != nil {
		opts = append(opts, withAgentMCP(mcp.DefaultRegistry.GetToolSchemas, mcp.DefaultRegistry))
	}
	return opts
}

func (m *model) ensureMemoryContextLoaded() {
	if m.cachedUserInstructions != "" || m.cachedProjectInstructions != "" {
		return
	}
	m.refreshMemoryContext("session_start")
}

// effectiveThinkingLevel returns the higher of the persistent level and the per-turn override.
func (m *model) effectiveThinkingLevel() llm.ThinkingLevel {
	return max(m.thinkingLevel, m.thinkingOverride)
}


func (m model) getModelID() string {
	if m.currentModel != nil {
		return m.currentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
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
	permBridge         *appoutput.PermissionBridge
	cancel             context.CancelFunc
	pendingPermRequest *appoutput.PermBridgeRequest
}

// buildCoreAgent creates a core.Agent and permissionBridge from the model's
// current state. The agent is not started — call startAgentLoop() for that.
func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.llmProvider == nil {
		return nil, errNoProvider
	}

	// LLM — wraps the current provider as core.LLM
	client := llm.NewClient(m.llmProvider, m.getModelID(), m.getMaxTokens())
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

	// Hooks — wrap hook.Engine as core.Hooks
	coreHooks := hook.AsCoreHooks(m.hookEngine)

	// Permission bridge — blocking PermissionFunc with TUI approval
	permBridge := appoutput.NewPermissionBridge(
		func() *setting.Settings { return m.settings },
		func() *setting.SessionPermissions { return m.sessionPermissions },
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
		appoutput.PollPermBridge(sess.permBridge),
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

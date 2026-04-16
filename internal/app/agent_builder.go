// agent_builder.go constructs a core.Agent from the TUI model's current state.
// This replaces buildLoopClient + buildLoopSystem + buildLoopToolSet for the
// agent-based path. The agent manages its own inference loop; the TUI observes
// events from the Outbox and sends user messages to the Inbox.
package app

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/messageconv"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

// agentSession holds the running core.Agent and its supporting infrastructure.
type agentSession struct {
	agent      core.Agent
	permBridge *permissionBridge
	cancel     context.CancelFunc
}

// buildCoreAgent creates a core.Agent and permissionBridge from the model's
// current state. The agent is not started — call startAgentLoop() for that.
func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.provider.LLM == nil {
		return nil, errNoProvider
	}

	// LLM — wraps the current provider as core.LLM
	llm := provider.NewLLM(m.provider.LLM, m.getModelID(), m.getMaxTokens())
	llm.SetThinking(m.effectiveThinkingLevel())

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
	permBridge := newPermissionBridge(
		func() *config.Settings { return m.settings },
		func() *config.SessionPermissions { return m.mode.SessionPermissions },
		func() string { return m.cwd },
	)

	ag := core.NewAgent(core.Config{
		ID:         "main",
		LLM:        llm,
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
		drainAgentOutbox(sess.agent.Outbox()),
		sess.permBridge.PollCmd(),
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

// shouldUseAgentPath returns true when the TUI should use the core.Agent
// execution path instead of the legacy streaming path. Currently controlled
// by the GEN_CORE_AGENT=1 environment variable for opt-in testing.
func (m *model) shouldUseAgentPath() bool {
	return os.Getenv("GEN_CORE_AGENT") == "1"
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
			coreMessages = append(coreMessages, messageconv.ToCore(msg))
		}
		sess.agent.SetMessages(coreMessages)
	}

	m.startAgentLoop(sess)
	return nil
}

var errNoProvider = providerRequiredError("no LLM provider configured")

type providerRequiredError string

func (e providerRequiredError) Error() string { return string(e) }

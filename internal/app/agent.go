// Agent session lifecycle: constructing, configuring, starting, stopping,
// and communicating with the core.Agent goroutine.
package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// ============================================================
// Agent session type
// ============================================================

type agentSession struct {
	agent              core.Agent
	permBridge         *conv.PermissionBridge
	cancel             context.CancelFunc
	pendingPermRequest *conv.PermBridgeRequest
}

var errNoProvider = providerRequiredError("no LLM provider configured")

type providerRequiredError string

func (e providerRequiredError) Error() string { return string(e) }

// ============================================================
// Agent construction and lifecycle
// ============================================================

func (m *model) buildCoreAgent() (*agentSession, error) {
	if m.env.LLMProvider == nil {
		return nil, errNoProvider
	}

	client := llm.NewClient(m.env.LLMProvider, m.env.GetModelID(), kit.GetMaxTokens(llm.Default().Store(), m.env.CurrentModel, setting.DefaultMaxTokens))
	client.SetThinking(m.env.EffectiveThinkingLevel())

	sys := m.buildSystemPrompt(nil, client)
	tools := m.buildAgentTools()

	permBridge := conv.NewPermissionBridge(func(name string, args map[string]any) conv.PermDecisionResult {
		settingSvc := setting.DefaultIfInit()
		if settingSvc == nil {
			return conv.PermDecisionResult{Decision: perm.Permit}
		}
		decision := settingSvc.HasPermissionToUseTool(name, args, m.env.SessionPermissions)
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

	ag := core.NewAgent(core.Config{
		ID:     "main",
		LLM:    client,
		System: sys,
		Tools:  tool.WithPermission(tools, permBridge.PermissionFunc()),
		CWD:    m.cwd,
	})

	return &agentSession{agent: ag, permBridge: permBridge}, nil
}

func (m *model) startAgentLoop(sess *agentSession) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	sess.cancel = cancel

	go func() {
		_ = sess.agent.Run(ctx)
	}()

	return tea.Batch(
		conv.DrainAgentOutbox(sess.agent.Outbox()),
		conv.PollPermBridge(sess.permBridge),
	)
}

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
	if sess.agent != nil {
		select {
		case sess.agent.Inbox() <- core.Message{Signal: core.SigStop}:
		default:
		}
	}
}

func (m *model) ensureAgentSession() (tea.Cmd, error) {
	if m.agentSess != nil {
		return nil, nil
	}
	sess, err := m.buildCoreAgent()
	if err != nil {
		return nil, err
	}
	m.agentSess = sess

	if len(m.conv.Messages) > 0 {
		var coreMessages []core.Message
		for _, msg := range m.conv.ConvertToProvider() {
			coreMessages = append(coreMessages, msg)
		}
		sess.agent.SetMessages(coreMessages)
	}

	return m.startAgentLoop(sess), nil
}

func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	inbox := m.agentSess.agent.Inbox()
	msg := core.Message{Role: core.RoleUser, Content: content, Images: images}
	return func() tea.Msg {
		inbox <- msg
		return nil
	}
}

func (m *model) StopAgentSession() {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
}

// ============================================================
// Agent outbox and permission bridge
// ============================================================

func (m *model) ContinueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return conv.DrainAgentOutbox(m.agentSess.agent.Outbox())
}

func (m *model) HandlePermBridge(req *conv.PermBridgeRequest) tea.Cmd {
	if m.agentSess != nil {
		m.agentSess.pendingPermRequest = req
	}
	if req == nil {
		return nil
	}
	m.userInput.Approval.Show(&perm.PermissionRequest{
		ToolName:    req.ToolName,
		Description: req.Description,
	}, m.width, m.height)
	return nil
}

// ============================================================
// Agent tool configuration
// ============================================================

func (m *model) ReconfigureAgentTool() {
	if m.env.LLMProvider == nil {
		return
	}
	m.ensureMemoryContextLoaded()

	executor := subagent.NewExecutor(m.env.LLMProvider, m.cwd, m.env.GetModelID(), hook.DefaultEngine)
	if session.Default().GetStore() != nil && session.Default().ID() != "" {
		executor.SetSessionStore(session.Default().GetStore(), session.Default().ID())
	}
	executor.SetContext(m.env.CachedUserInstructions, m.env.CachedProjectInstructions, m.isGit)
	if mcp.DefaultRegistry != nil {
		executor.SetMCP(mcp.DefaultRegistry.GetToolSchemas, mcp.DefaultRegistry)
	}

	adapter := subagent.NewExecutorAdapter(executor)
	type executorSetter interface{ SetExecutor(tool.AgentExecutor) }
	for _, name := range []string{tool.ToolAgent, tool.ToolContinueAgent, tool.ToolSendMessage} {
		if t, ok := tool.Get(name); ok {
			if setter, ok := t.(executorSetter); ok {
				setter.SetExecutor(adapter)
			}
		}
	}
}

// ============================================================
// System prompt and tool set
// ============================================================

func (m *model) buildSystemPrompt(extra []string, loopClient *llm.Client) core.System {
	var providerName, modelID string
	if loopClient != nil {
		modelID = loopClient.ModelID()
		providerName = loopClient.Name()
	}

	allExtra := append([]string{}, extra...)
	if coordinator := system.CoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.userInput.Skill.ActiveInvocation)
	}

	var sessionSummary string
	if summary := session.Default().GetSummary(); summary != "" {
		sessionSummary = fmt.Sprintf("<session-summary>\n%s\n</session-summary>", summary)
	}

	skills := skill.Default().PromptSection()
	agents := subagent.Default().PromptSection()

	return system.Build(system.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.env.PlanEnabled,
		UserInstructions:    m.env.CachedUserInstructions,
		ProjectInstructions: m.env.CachedProjectInstructions,
		SessionSummary:      sessionSummary,
		Skills:              skills,
		Agents:              agents,
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               allExtra,
	})
}

func (m *model) buildAgentTools() core.Tools {
	var mcpGetter func() []core.ToolSchema
	if mcp.DefaultRegistry != nil {
		mcpGetter = mcp.DefaultRegistry.GetToolSchemas
	}
	schemas := (&tool.Set{
		Disabled: setting.Default().DisabledTools(),
		PlanMode: m.env.PlanEnabled,
		MCP:      mcpGetter,
	}).Tools()

	tools := tool.AdaptToolRegistry(schemas, func() string { return m.cwd })
	if mcp.DefaultRegistry != nil {
		mcpCaller := mcp.NewCaller(mcp.DefaultRegistry)
		for _, t := range mcp.AsCoreTools(schemas, mcpCaller) {
			tools.Add(t)
		}
	}
	return tools
}

// ============================================================
// LLM client
// ============================================================

func (m *model) buildLLMClient() *llm.Client {
	c := llm.NewClient(m.env.LLMProvider, m.env.GetModelID(), kit.GetMaxTokens(llm.Default().Store(), m.env.CurrentModel, setting.DefaultMaxTokens))
	c.SetThinking(m.env.EffectiveThinkingLevel())
	return c
}

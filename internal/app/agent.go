// Agent session lifecycle: building params, delegating to agent.Service,
// and wrapping channels in tea.Cmds for the TUI.
package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// ============================================================
// Build params from model state
// ============================================================

func (m *model) buildAgentParams() agent.BuildParams {
	var sessionSummary string
	if summary := m.services.Session.GetSummary(); summary != "" {
		sessionSummary = fmt.Sprintf("<session-summary>\n%s\n</session-summary>", summary)
	}

	var extra []string
	if coordinator := system.CoordinatorGuidance(); coordinator != "" {
		extra = append(extra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		extra = append(extra, m.userInput.Skill.ActiveInvocation)
	}

	var mcpTools []core.Tool
	if m.services.MCP.Registry() != nil {
		schemas := m.services.MCP.Registry().GetToolSchemas()
		mcpCaller := mcp.NewCaller(m.services.MCP.Registry())
		mcpTools = mcp.AsCoreTools(schemas, mcpCaller)
	}

	return agent.BuildParams{
		Provider:      m.env.LLMProvider,
		ModelID:       m.env.GetModelID(),
		MaxTokens:     kit.GetMaxTokens(m.services.LLM.Store(), m.env.CurrentModel, setting.DefaultMaxTokens),
		ThinkingLevel: m.env.EffectiveThinkingLevel(),

		CWD:     m.cwd,
		CWDFunc: func() string { return m.cwd },
		IsGit:   m.isGit,

		PlanEnabled:         m.env.PlanEnabled,
		UserInstructions:    m.env.CachedUserInstructions,
		ProjectInstructions: m.env.CachedProjectInstructions,
		SessionSummary:      sessionSummary,
		SkillsPrompt:        m.services.Skill.PromptSection(),
		AgentsPrompt:        m.services.Subagent.PromptSection(),
		DeferredToolsPrompt: m.services.Tool.FormatDeferredToolsPrompt(),
		Extra:               extra,

		DisabledTools: m.services.Setting.DisabledTools(),
		MCPTools:      mcpTools,

		PermissionDecider: func(name string, args map[string]any) agent.PermDecisionResult {
			decision := m.services.Setting.HasPermissionToUseTool(name, args, m.env.SessionPermissions)
			switch decision.Behavior {
			case setting.Allow:
				return agent.PermDecisionResult{Decision: perm.Permit, Reason: decision.Reason}
			case setting.Deny:
				return agent.PermDecisionResult{Decision: perm.Reject, Reason: decision.Reason}
			default:
				return agent.PermDecisionResult{
					Decision:    perm.Prompt,
					Reason:      decision.Reason,
					ToolName:    name,
					Description: decision.Reason,
				}
			}
		},
	}
}

// ============================================================
// Agent lifecycle (delegates to services.Agent)
// ============================================================

func (m *model) ensureAgentSession() (tea.Cmd, error) {
	if m.services.Agent.Active() {
		return nil, nil
	}

	params := m.buildAgentParams()

	var coreMessages []core.Message
	if len(m.conv.Messages) > 0 {
		for _, msg := range m.conv.ConvertToProvider() {
			coreMessages = append(coreMessages, msg)
		}
	}

	if err := m.services.Agent.Start(params, coreMessages); err != nil {
		return nil, err
	}

	return tea.Batch(
		conv.DrainAgentOutbox(m.services.Agent.Outbox()),
		conv.PollPermBridge(m.services.Agent.PermissionBridge()),
	), nil
}

func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if !m.services.Agent.Active() {
		return nil
	}
	svc := m.services.Agent
	return func() tea.Msg {
		svc.Send(content, images)
		return nil
	}
}

func (m *model) StopAgentSession() {
	m.services.Agent.Stop()
}

// ============================================================
// Agent outbox and permission bridge
// ============================================================

func (m *model) ContinueOutbox() tea.Cmd {
	if !m.services.Agent.Active() {
		return nil
	}
	return conv.DrainAgentOutbox(m.services.Agent.Outbox())
}

func (m *model) HandlePermBridge(req *conv.PermBridgeRequest) tea.Cmd {
	m.services.Agent.SetPendingPermission(req)
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

	var hookEngine *hook.Engine
	if m.services.Hook != nil {
		hookEngine = m.services.Hook.Engine()
	}
	executor := subagent.NewExecutor(m.env.LLMProvider, m.cwd, m.env.GetModelID(), hookEngine)
	if m.services.Session.GetStore() != nil && m.services.Session.ID() != "" {
		executor.SetSessionStore(m.services.Session.GetStore(), m.services.Session.ID())
	}
	executor.SetContext(m.env.CachedUserInstructions, m.env.CachedProjectInstructions, m.isGit)
	if m.services.MCP.Registry() != nil {
		executor.SetMCP(m.services.MCP.Registry().GetToolSchemas, m.services.MCP.Registry())
	}

	adapter := subagent.NewExecutorAdapter(executor)
	type executorSetter interface{ SetExecutor(tool.AgentExecutor) }
	for _, name := range []string{tool.ToolAgent, tool.ToolContinueAgent, tool.ToolSendMessage} {
		if t, ok := m.services.Tool.Get(name); ok {
			if setter, ok := t.(executorSetter); ok {
				setter.SetExecutor(adapter)
			}
		}
	}
}

// ============================================================
// LLM client
// ============================================================

func (m *model) buildLLMClient() *llm.Client {
	c := llm.NewClient(m.env.LLMProvider, m.env.GetModelID(), kit.GetMaxTokens(m.services.LLM.Store(), m.env.CurrentModel, setting.DefaultMaxTokens))
	c.SetThinking(m.env.EffectiveThinkingLevel())
	return c
}

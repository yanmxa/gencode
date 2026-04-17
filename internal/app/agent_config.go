// LLM loop configuration and agent tool wiring.
// These methods assemble the system prompt, tool set, and subagent executor
// from the model's current state for use by core.Agent and the agent tool.
package app

import (
	"fmt"

	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/tool"
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
)

func (m *model) buildLoopClient() *llm.Client {
	c := llm.NewClient(m.runtime.LLMProvider, m.getModelID(), m.getMaxTokens())
	c.SetThinking(m.effectiveThinkingLevel())
	return c
}

func (m *model) buildLoopSystem(extra []string, loopClient *llm.Client) core.System {
	providerName := ""
	modelID := ""
	if loopClient != nil {
		modelID = loopClient.ModelID()
		providerName = loopClient.Name()
	}
	return system.Build(system.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.runtime.PlanEnabled,
		UserInstructions:    m.runtime.CachedUserInstructions,
		ProjectInstructions: m.runtime.CachedProjectInstructions,
		SessionSummary:      m.buildSessionSummaryBlock(),
		Skills:              m.buildLoopSkillsSection(),
		Agents:              m.buildLoopAgentsSection(),
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               m.buildLoopExtra(extra),
	})
}

func (m *model) buildLoopToolSet() *tool.Set {
	return &tool.Set{
		Disabled: m.runtime.DisabledTools,
		PlanMode: m.runtime.PlanEnabled,
		MCP:      m.buildMCPToolsGetter(),
	}
}

func (m *model) buildLoopExtra(extra []string) []string {
	allExtra := append([]string{}, extra...)
	if coordinator := buildCoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.userInput.Skill.ActiveInvocation)
	}
	return allExtra
}

func buildCoordinatorGuidance() string {
	return system.CoordinatorGuidance()
}

func (m *model) buildSessionSummaryBlock() string {
	if m.runtime.SessionSummary == "" {
		return ""
	}
	return fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.runtime.SessionSummary)
}

func (m *model) buildLoopSkillsSection() string {
	if skill.DefaultRegistry == nil {
		return ""
	}
	return skill.DefaultRegistry.GetSkillsSection()
}

func (m *model) buildLoopAgentsSection() string {
	if subagent.DefaultRegistry == nil {
		return ""
	}
	return subagent.DefaultRegistry.GetAgentsSection()
}

func (m *model) buildMCPToolsGetter() func() []core.ToolSchema {
	if mcp.DefaultRegistry == nil {
		return nil
	}
	return mcp.DefaultRegistry.GetToolSchemas
}

// --- Agent tool wiring ---

type agentToolOption func(*subagent.Executor)

func configureAgentTool(llmProvider llm.Provider, cwd string, modelID string, hookEngine *hook.Engine, sessionStore *session.Store, parentSessionID string, opts ...agentToolOption) {
	executor := subagent.NewExecutor(llmProvider, cwd, modelID, hookEngine)
	if sessionStore != nil && parentSessionID != "" {
		executor.SetSessionStore(sessionStore, parentSessionID)
	}
	for _, opt := range opts {
		opt(executor)
	}
	adapter := subagent.NewExecutorAdapter(executor)

	if t, ok := tool.Get(tool.ToolAgent); ok {
		if agentTool, ok := t.(*toolagent.AgentTool); ok {
			agentTool.SetExecutor(adapter)
		}
	}
	if t, ok := tool.Get(tool.ToolContinueAgent); ok {
		if continueTool, ok := t.(*toolagent.ContinueAgentTool); ok {
			continueTool.SetExecutor(adapter)
		}
	}
	if t, ok := tool.Get(tool.ToolSendMessage); ok {
		if sendMessageTool, ok := t.(*toolagent.SendMessageTool); ok {
			sendMessageTool.SetExecutor(adapter)
		}
	}
}

func withAgentContext(userInstructions, projectInstructions string, isGit bool) agentToolOption {
	return func(e *subagent.Executor) {
		e.SetContext(userInstructions, projectInstructions, isGit)
	}
}

func withAgentMCP(getter func() []core.ToolSchema, registry *mcp.Registry) agentToolOption {
	return func(e *subagent.Executor) {
		e.SetMCP(getter, registry)
	}
}

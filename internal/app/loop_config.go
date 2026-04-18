package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
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
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
)

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

func (m *model) getEffectiveInputLimit() int {
	return kit.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return kit.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens)
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

func (m *model) buildLoopClient() *llm.Client {
	c := llm.NewClient(m.runtime.LLMProvider, m.getModelID(), m.getMaxTokens())
	c.SetThinking(m.effectiveThinkingLevel())
	return c
}

func (m *model) buildPromptSuggestionRequest() (input.PromptSuggestionRequest, bool) {
	return input.BuildPromptSuggestionRequest(input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv,
		Runtime:      &m.runtime,
		BuildClient:  m.buildLoopClient,
	})
}

func (m *model) startPromptSuggestion() tea.Cmd {
	return input.StartPromptSuggestion(input.PromptSuggestionDeps{
		Input:        &m.userInput,
		Conversation: &m.conv,
		Runtime:      &m.runtime,
		BuildClient:  m.buildLoopClient,
	})
}

func (m *model) buildCompactRequest(focus, trigger string) conv.CompactRequest {
	return conv.CompactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLoopClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.runtime.SessionSummary,
		Focus:          focus,
		HookEngine:     m.runtime.HookEngine,
		Trigger:        trigger,
	}
}

func (m *model) saveSession() error {
	return appruntime.SaveSession(appruntime.SessionSaveDeps{
		Runtime:         &m.runtime,
		Cwd:             m.cwd,
		Messages:        m.conv.Messages,
		ReconfigureTool: m.reconfigureAgentTool,
	})
}

func (m *model) loadSession(id string) error {
	return appruntime.LoadSession(appruntime.SessionLoadDeps{
		Runtime: &m.runtime,
		Cwd:     m.cwd,
		RestoreMessages: func(msgs []core.ChatMessage) {
			m.conv.Messages = msgs
		},
	}, id)
}

func (m *model) initTaskStorage() {
	appruntime.InitTaskStorage(m.runtime.SessionID)
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	appruntime.RestoreSessionData(&m.runtime, sess, func(msgs []core.ChatMessage) {
		m.conv.Messages = msgs
	})
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
	if coordinator := system.CoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.userInput.Skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.userInput.Skill.ActiveInvocation)
	}
	return allExtra
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

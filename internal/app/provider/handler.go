package provider

import (
	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
)

// ConfigureAgentTool sets up the Agent tool with a subagent executor
// backed by the given LLM provider.
func ConfigureAgentTool(llmProvider coreprovider.LLMProvider, cwd string, modelID string, hookEngine *hooks.Engine, sessionStore *session.Store, parentSessionID string, opts ...AgentToolOption) {
	if t, ok := tool.Get("Agent"); ok {
		if agentTool, ok := t.(*tool.AgentTool); ok {
			executor := agent.NewExecutor(llmProvider, cwd, modelID, hookEngine)
			if sessionStore != nil && parentSessionID != "" {
				executor.SetSessionStore(sessionStore, parentSessionID)
			}
			// Apply options (e.g., project context, MCP)
			for _, opt := range opts {
				opt(executor)
			}
			adapter := agent.NewExecutorAdapter(executor)
			agentTool.SetExecutor(adapter)
		}
	}
}

// AgentToolOption configures the agent executor.
type AgentToolOption func(*agent.Executor)

// WithContext provides project instructions and git status to subagents.
func WithContext(userInstructions, projectInstructions string, isGit bool) AgentToolOption {
	return func(e *agent.Executor) {
		e.SetContext(userInstructions, projectInstructions, isGit)
	}
}

// WithMCP provides the parent's MCP tool getter and registry so subagents
// can access MCP tools (schemas for tool list, registry for execution).
func WithMCP(getter func() []coreprovider.Tool, registry *mcp.Registry) AgentToolOption {
	return func(e *agent.Executor) {
		e.SetMCP(getter, registry)
	}
}

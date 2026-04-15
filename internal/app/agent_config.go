package app

import (
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
	toolagent "github.com/yanmxa/gencode/internal/tool/agent"
)

type agentToolOption func(*subagent.Executor)

func configureAgentTool(llmProvider provider.LLMProvider, cwd string, modelID string, hookEngine *hooks.Engine, sessionStore *session.Store, parentSessionID string, opts ...agentToolOption) {
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

func withAgentMCP(getter func() []provider.ToolSchema, registry *mcp.Registry) agentToolOption {
	return func(e *subagent.Executor) {
		e.SetMCP(getter, registry)
	}
}

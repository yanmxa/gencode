package provider

import (
	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/hooks"
	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
)

// ConfigureTaskTool sets up the Task tool with a subagent executor
// backed by the given LLM provider.
func ConfigureTaskTool(llmProvider coreprovider.LLMProvider, cwd string, modelID string, hookEngine *hooks.Engine, sessionStore *session.Store, parentSessionID string) {
	if t, ok := tool.Get("Task"); ok {
		if taskTool, ok := t.(*tool.TaskTool); ok {
			executor := agent.NewExecutor(llmProvider, cwd, modelID, hookEngine)
			if sessionStore != nil && parentSessionID != "" {
				executor.SetSessionStore(sessionStore, parentSessionID)
			}
			adapter := agent.NewExecutorAdapter(executor)
			taskTool.SetExecutor(adapter)
		}
	}
}

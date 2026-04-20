package tool

import "github.com/yanmxa/gencode/internal/core"

// Tool name constants used in runtime comparisons across the codebase.
const (
	ToolAgent         = "Agent"
	ToolContinueAgent = "ContinueAgent"
	ToolSendMessage   = "SendMessage"
	ToolTaskOutput    = "TaskOutput"
	ToolTaskStop      = "TaskStop"

	// Deprecated aliases — kept for backward compatibility with cached model contexts.
	ToolAgentOutput   = ToolTaskOutput
	ToolAgentStop     = ToolTaskStop
	ToolSkill      = "Skill"
	ToolTaskCreate = "TaskCreate"
	ToolTaskGet       = "TaskGet"
	ToolTaskUpdate    = "TaskUpdate"
	ToolTaskList      = "TaskList"
	ToolCronCreate    = "CronCreate"
	ToolCronDelete    = "CronDelete"
	ToolCronList      = "CronList"
	ToolEnterWorktree = "EnterWorktree"
	ToolExitWorktree  = "ExitWorktree"
	ToolToolSearch    = "ToolSearch"

	ToolAskUserQuestion = "AskUserQuestion"
)

// IsAgentToolName reports whether the tool name represents an agent-like worker tool.
func IsAgentToolName(name string) bool {
	return name == ToolAgent || name == ToolContinueAgent || name == ToolSendMessage
}

// GetToolSchemas returns core.ToolSchema definitions for all registered tools
func GetToolSchemas() []core.ToolSchema {
	return GetToolSchemasWithMCP(nil)
}

// GetToolSchemasWithMCP returns tool schemas including MCP tools if a getter is provided
func GetToolSchemasWithMCP(mcpToolsGetter func() []core.ToolSchema) []core.ToolSchema {
	tools := make([]core.ToolSchema, 0, 20)
	tools = append(tools, baseToolSchemas()...)

	// Add Skill tool
	tools = append(tools, skillToolSchema)

	// Add Agent tool
	tools = append(tools, agentToolSchema)
	tools = append(tools, continueAgentToolSchema)
	tools = append(tools, sendMessageToolSchema)

	// Add ToolSearch (always available — enables progressive disclosure)
	tools = append(tools, toolSearchSchema)

	// Add Tracker tools
	tools = append(tools, trackerToolSchemas...)

	// Add Cron tools
	tools = append(tools, cronToolSchemas...)

	// Add Worktree tools
	tools = append(tools, worktreeToolSchemas...)

	// Add MCP tools if getter is provided
	if mcpToolsGetter != nil {
		tools = append(tools, mcpToolsGetter()...)
	}

	return tools
}

func filterSchemas(all []core.ToolSchema, disabled map[string]bool) []core.ToolSchema {
	if len(disabled) == 0 {
		return all
	}
	filtered := make([]core.ToolSchema, 0, len(all))
	for _, t := range all {
		if !disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

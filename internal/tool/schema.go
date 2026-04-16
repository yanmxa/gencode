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
	ToolSkill         = "Skill"
	ToolEnterPlanMode = "EnterPlanMode"
	ToolExitPlanMode  = "ExitPlanMode"
	ToolTaskCreate    = "TaskCreate"
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

	// Add EnterPlanMode to normal mode tools
	tools = append(tools, enterPlanModeSchema)

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

// getPlanModeToolSchemas returns only the tools available in plan mode.
// Plan mode restricts to read-only tools, the plan-mode Agent tool, plus ExitPlanMode.
func getPlanModeToolSchemas() []core.ToolSchema {
	// Read-only tools allowed in plan mode
	allowedTools := map[string]bool{
		"Read":            true,
		"Glob":            true,
		"Grep":            true,
		"WebFetch":        true,
		"WebSearch":       true,
		"AskUserQuestion": true, // allow LLM to ask clarifying questions in plan mode
	}

	// Filter to allowed read-only tools
	allTools := GetToolSchemas()
	tools := make([]core.ToolSchema, 0, len(allowedTools)+2)

	for _, t := range allTools {
		if allowedTools[t.Name] {
			tools = append(tools, t)
		}
	}

	// Add plan-mode Agent schema (no run_in_background, restricted agent types)
	tools = append(tools, planModeAgentSchema)

	// Add ExitPlanMode
	tools = append(tools, exitPlanModeSchema)

	return tools
}

// getPlanModeToolSchemasFiltered returns plan mode tools excluding disabled tools
func getPlanModeToolSchemasFiltered(disabled map[string]bool) []core.ToolSchema {
	return filterSchemas(getPlanModeToolSchemas(), disabled)
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

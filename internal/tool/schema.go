package tool

import "github.com/yanmxa/gencode/internal/provider"

// Tool name constants used in runtime comparisons across the codebase.
const (
	ToolAgent      = "Agent"
	ToolTaskOutput = "TaskOutput"
	ToolTaskStop   = "TaskStop"

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
)

// ToolSchema defines the JSON schema for a tool
type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// GetToolSchemas returns provider.Tool definitions for all registered tools
func GetToolSchemas() []provider.Tool {
	return GetToolSchemasWithMCP(nil)
}

// GetToolSchemasWithMCP returns tool schemas including MCP tools if a getter is provided
func GetToolSchemasWithMCP(mcpToolsGetter func() []provider.Tool) []provider.Tool {
	tools := baseToolSchemas()

	// Add EnterPlanMode to normal mode tools
	tools = append(tools, EnterPlanModeSchema)

	// Add Skill tool
	tools = append(tools, SkillToolSchema)

	// Add Agent tool
	tools = append(tools, AgentToolSchema)

	// Add ToolSearch (always available — enables progressive disclosure)
	tools = append(tools, ToolSearchSchema)

	// Add Todo tools
	tools = append(tools, TodoToolSchemas...)

	// Add Cron tools
	tools = append(tools, CronToolSchemas...)

	// Add Worktree tools
	tools = append(tools, WorktreeToolSchemas...)

	// Add MCP tools if getter is provided
	if mcpToolsGetter != nil {
		tools = append(tools, mcpToolsGetter()...)
	}

	return tools
}

// GetToolSchemasFiltered returns tool schemas excluding disabled tools
func GetToolSchemasFiltered(disabled map[string]bool) []provider.Tool {
	all := GetToolSchemas()
	if len(disabled) == 0 {
		return all
	}
	filtered := make([]provider.Tool, 0, len(all))
	for _, t := range all {
		if !disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// GetPlanModeToolSchemas returns only the tools available in plan mode
// Plan mode restricts to read-only tools, the plan-specific Task tool, plus ExitPlanMode
func GetPlanModeToolSchemas() []provider.Tool {
	// Read-only tools allowed in plan mode
	allowedTools := map[string]bool{
		"Read":      true,
		"Glob":      true,
		"Grep":      true,
		"WebFetch":  true,
		"WebSearch": true,
	}

	// Filter to allowed read-only tools
	allTools := GetToolSchemas()
	tools := make([]provider.Tool, 0, len(allowedTools)+2)

	for _, t := range allTools {
		if allowedTools[t.Name] {
			tools = append(tools, t)
		}
	}

	// Add plan-mode Agent schema (no run_in_background, restricted agent types)
	tools = append(tools, PlanModeAgentSchema)

	// Add ExitPlanMode
	tools = append(tools, ExitPlanModeSchema)

	return tools
}

// GetPlanModeToolSchemasFiltered returns plan mode tools excluding disabled tools
func GetPlanModeToolSchemasFiltered(disabled map[string]bool) []provider.Tool {
	all := GetPlanModeToolSchemas()
	if len(disabled) == 0 {
		return all
	}
	filtered := make([]provider.Tool, 0, len(all))
	for _, t := range all {
		if !disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

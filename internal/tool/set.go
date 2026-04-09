package tool

import (
	"strings"

	"github.com/yanmxa/gencode/internal/provider"
)

// parentOnlyTools are tools that only the parent conversation can use.
// Subagents never get these regardless of their allow list.
var parentOnlyTools = map[string]bool{
	ToolEnterPlanMode: true,
	ToolExitPlanMode:  true,
	ToolEnterWorktree: true,
	ToolExitWorktree:  true,
}

// Set provides tools for a conversation turn.
// If Static is non-nil, it is returned directly (for custom agents).
// Otherwise, tools are resolved dynamically using the config fields.
type Set struct {
	Static    []provider.ToolSchema        // fixed tool list (overrides dynamic)
	Disabled  map[string]bool              // excluded tools
	PlanMode  bool                         // plan mode filter
	MCP       func() []provider.ToolSchema // MCP tools getter
	Allow     []string                     // agent allow list (nil = all tools, non-nil = only these)
	Disallow  []string                     // agent deny list (excluded after allow filtering)
	IsAgent   bool                         // true for subagent tool sets (excludes parent-only tools)
}

// Tools returns the resolved tool set for a turn.
func (s *Set) Tools() []provider.ToolSchema {
	// Static tools override everything
	if s.Static != nil {
		return s.Static
	}

	// Agent with explicit allow list
	if s.Allow != nil {
		return s.agentTools()
	}

	// Agent with nil allow = all tools minus parent-only
	if s.IsAgent {
		return s.agentAllTools()
	}

	// Default mode: full set with disabled/plan filtering
	return s.defaultTools()
}

// defaultTools returns the full tool set filtered by disabled/plan/deferred mode.
func (s *Set) defaultTools() []provider.ToolSchema {
	if s.PlanMode {
		return GetPlanModeToolSchemasFiltered(s.Disabled)
	}

	tools := GetToolSchemasWithMCP(s.MCP)

	filtered := make([]provider.ToolSchema, 0, len(tools))
	for _, t := range tools {
		if s.Disabled[t.Name] {
			continue
		}
		// Skip deferred tools unless they've been fetched via ToolSearch
		if IsDeferred(t.Name) && !IsFetched(t.Name) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// agentAllTools returns all tools except parent-only and disallowed tools.
// Used for agents with nil Allow (= all tools).
func (s *Set) agentAllTools() []provider.ToolSchema {
	allTools := GetToolSchemasWithMCP(s.MCP)
	filtered := make([]provider.ToolSchema, 0, len(allTools))
	for _, t := range allTools {
		if !parentOnlyTools[t.Name] && !s.isDisallowed(t.Name) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// agentTools returns tools filtered by the allow list.
// Only tools in the Allow list are included. MCP tools matching
// the allow list (e.g. "mcp__server__tool") are also included.
func (s *Set) agentTools() []provider.ToolSchema {
	allTools := GetToolSchemas()

	// Build allow set for fast lookup
	allowSet := make(map[string]bool, len(s.Allow))
	for _, name := range s.Allow {
		allowSet[strings.ToLower(name)] = true
	}

	filtered := make([]provider.ToolSchema, 0, len(s.Allow))
	for _, t := range allTools {
		if allowSet[strings.ToLower(t.Name)] && !s.isDisallowed(t.Name) {
			filtered = append(filtered, t)
		}
	}

	// Include MCP tools that match the allow list
	if s.MCP != nil {
		for _, t := range s.MCP() {
			if allowSet[strings.ToLower(t.Name)] && !s.isDisallowed(t.Name) {
				filtered = append(filtered, t)
			}
		}
	}

	return filtered
}

// isDisallowed checks if a tool name is in the Disallow list.
func (s *Set) isDisallowed(name string) bool {
	lower := strings.ToLower(name)
	for _, d := range s.Disallow {
		if strings.ToLower(d) == lower {
			return true
		}
	}
	return false
}

package tool

import (
	"strings"

	"github.com/yanmxa/gencode/internal/provider"
)

// AccessMode controls how tool access is configured for agents.
type AccessMode string

const (
	// AccessAllowlist only allows specified tools.
	AccessAllowlist AccessMode = "allowlist"
	// AccessDenylist allows all except specified tools.
	AccessDenylist AccessMode = "denylist"
)

// AccessConfig configures agent allow/deny lists.
type AccessConfig struct {
	Mode  AccessMode
	Allow []string
	Deny  []string
}

// Set provides tools for a conversation turn.
// If Static is non-nil, it is returned directly (for custom agents).
// Otherwise, tools are resolved dynamically using the config fields.
type Set struct {
	Static   []provider.Tool        // fixed tool list (overrides dynamic)
	Disabled map[string]bool        // excluded tools
	PlanMode bool                   // plan mode filter
	MCP      func() []provider.Tool // MCP tools getter
	Access   *AccessConfig          // agent allow/deny lists
}

// Tools returns the resolved tool set for a turn.
func (s *Set) Tools() []provider.Tool {
	// Static tools override everything
	if s.Static != nil {
		return s.Static
	}

	// Agent mode: filtered by allow/deny lists
	if s.Access != nil {
		return s.agentTools()
	}

	// Default mode: full set with disabled/plan filtering
	return s.defaultTools()
}

// defaultTools returns the full tool set filtered by disabled/plan mode.
func (s *Set) defaultTools() []provider.Tool {
	if s.PlanMode {
		return GetPlanModeToolSchemasFiltered(s.Disabled)
	}

	tools := GetToolSchemasWithMCP(s.MCP)

	if len(s.Disabled) == 0 {
		return tools
	}
	filtered := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		if !s.Disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// agentBlockedTools are tools that agents cannot use.
var agentBlockedTools = map[string]bool{
	"Task":          true, // prevents nested spawning
	"EnterPlanMode": true, // plan mode is parent-only
	"ExitPlanMode":  true,
}

// agentTools returns tools filtered by agent allow/deny lists.
func (s *Set) agentTools() []provider.Tool {
	allTools := GetToolSchemas()
	filtered := make([]provider.Tool, 0, len(allTools))

	for _, t := range allTools {
		if agentBlockedTools[t.Name] {
			continue
		}

		if !s.isToolAllowed(t.Name) {
			continue
		}

		filtered = append(filtered, t)
	}

	return filtered
}

// isToolAllowed checks if a tool is allowed by the access config.
func (s *Set) isToolAllowed(name string) bool {
	switch s.Access.Mode {
	case AccessAllowlist:
		for _, allowed := range s.Access.Allow {
			if strings.EqualFold(name, allowed) {
				return true
			}
		}
		return false
	case AccessDenylist:
		for _, denied := range s.Access.Deny {
			if strings.EqualFold(name, denied) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

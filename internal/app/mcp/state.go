package mcp

import "github.com/yanmxa/gencode/internal/mcp"

// State holds all MCP-related state for the TUI model.
type State struct {
	Selector Model
	Registry *mcp.Registry
}

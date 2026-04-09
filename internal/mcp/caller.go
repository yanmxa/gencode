package mcp

import (
	"context"
	"fmt"
	"strings"
)

// Caller wraps a Registry to implement runtime.MCPCaller without import cycles.
type Caller struct {
	registry *Registry
}

// NewCaller creates an MCP caller from a registry.
func NewCaller(reg *Registry) *Caller {
	return &Caller{registry: reg}
}

// IsMCPTool returns true if the name is an MCP tool (mcp__*__*).
func (c *Caller) IsMCPTool(name string) bool {
	return IsMCPTool(name)
}

// CallTool calls an MCP tool and returns the content string and error status.
func (c *Caller) CallTool(ctx context.Context, fullName string, arguments map[string]any) (string, bool, error) {
	result, err := c.registry.CallTool(ctx, fullName, arguments)
	if err != nil {
		return "", false, err
	}

	content := extractContent(result.Content)
	return content, result.IsError, nil
}

// extractContent extracts text content from MCP tool result.
func extractContent(contents []ToolResultContent) string {
	var parts []string
	for _, c := range contents {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ConnectServers connects to a specific set of MCP servers from the registry.
// Returns a cleanup function that disconnects them.
func ConnectServers(ctx context.Context, reg *Registry, serverNames []string) (cleanup func(), errs []error) {
	var connected []string
	for _, name := range serverNames {
		if _, ok := reg.GetConfig(name); !ok {
			errs = append(errs, fmt.Errorf("MCP server not configured: %s", name))
			continue
		}
		if err := reg.Connect(ctx, name); err != nil {
			errs = append(errs, fmt.Errorf("MCP server %s: %w", name, err))
			continue
		}
		connected = append(connected, name)
	}

	cleanup = func() {
		for _, name := range connected {
			_ = reg.Disconnect(name)
		}
	}
	return cleanup, errs
}

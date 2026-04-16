// handler_command_config.go contains configuration and settings command handlers:
// /provider, /model, /init, /memory, /mcp, /plugin, and /reload-plugins.
package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/extension/plugin"
)

func handleSearchCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.search.Selector.Enter(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleModelCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	cmd, err := m.provider.Selector.Enter(ctx, m.width, m.height)
	if err != nil {
		return "", nil, err
	}
	return "", cmd, nil
}

func handleInitCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, err := appmemory.HandleInitCommand(m.cwd, args)
	return result, nil, err
}

func handleMemoryCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, editPath, err := appmemory.HandleMemoryCommand(&m.memory.Selector, m.cwd, m.width, m.height, args)
	if err != nil {
		return "", nil, err
	}
	if editPath != "" {
		m.memory.EditingFile = editPath
		return result, startExternalEditor(editPath), nil
	}
	return result, nil, nil
}

func handleMCPCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, editInfo, err := mcpui.HandleCommand(ctx, &m.mcp.Selector, m.width, m.height, args)
	if err != nil {
		return "", nil, err
	}
	if editInfo != nil {
		m.mcp.EditingFile = editInfo.TempFile
		m.mcp.EditingServer = editInfo.ServerName
		m.mcp.EditingScope = editInfo.Scope
		return result, mcpui.StartMCPEditor(editInfo.TempFile), nil
	}
	if m.mcp.Selector.IsActive() {
		return result, m.mcp.Selector.AutoReconnect(), nil
	}
	return result, nil, nil
}

func handlePluginCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, err := pluginui.HandleCommand(ctx, &m.plugin.Selector, m.cwd, m.width, m.height, args)
	return result, nil, err
}

func handleReloadPluginsCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if strings.TrimSpace(args) != "" {
		return "Usage: /reload-plugins", nil, nil
	}

	if err := plugin.DefaultRegistry.Load(ctx, m.cwd); err != nil {
		return "", nil, fmt.Errorf("failed to reload plugin registry: %w", err)
	}
	_ = plugin.DefaultRegistry.LoadClaudePlugins(ctx)

	if err := m.reloadPluginBackedState(); err != nil {
		return "", nil, err
	}

	return "Reloaded plugins and refreshed plugin-backed skills, agents, MCP servers, and hooks.", nil, nil
}

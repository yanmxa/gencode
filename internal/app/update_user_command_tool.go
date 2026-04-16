// handler_command_tool.go contains tool and utility command handlers:
// /tools, /glob, /skills, and /agents.
package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/output/render"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/extension/mcp"
	"github.com/yanmxa/gencode/internal/tool"
)

func handleGlobCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil, nil
	}

	params := map[string]any{"pattern": args}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "glob", params, m.cwd)
	return render.RenderToolResult(result, m.width), nil, nil
}

func handleToolCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	var mcpTools func() []core.ToolSchema
	if mcp.DefaultRegistry != nil {
		mcpTools = mcp.DefaultRegistry.GetToolSchemas
	}
	if err := m.tool.Selector.EnterSelect(m.width, m.height, m.disabledTools, mcpTools); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleSkillCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.skill.Selector.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleAgentCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.agent.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

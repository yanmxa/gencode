package tui

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
	toolui "github.com/yanmxa/gencode/internal/tool/ui"
)

type (
	startToolExecutionMsg struct {
		toolCalls []provider.ToolCall
	}
	allToolsCompletedMsg struct{}
	toolResultMsg        struct {
		result   provider.ToolResult
		toolName string
	}
	todoResultMsg struct {
		result   provider.ToolResult
		toolName string
		todos    []toolui.TodoItem
	}
)

func (m model) executeTools(toolCalls []provider.ToolCall) tea.Cmd {
	return func() tea.Msg {
		return startToolExecutionMsg{toolCalls: toolCalls}
	}
}

func processNextTool(toolCalls []provider.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	if idx >= len(toolCalls) {
		return func() tea.Msg {
			return allToolsCompletedMsg{}
		}
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Unknown tool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			switch permResult {
			case config.PermissionAllow:
				result := tool.Execute(ctx, tc.Name, params, cwd)
				return toolResultMsg{
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
					toolName: tc.Name,
				}
			case config.PermissionDeny:
				return toolResultMsg{
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Permission denied by settings", IsError: true},
					toolName: tc.Name,
				}
			case config.PermissionAsk:
				// Fall through
			}
		}

		if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
			req, err := it.PrepareInteraction(ctx, params, cwd)
			if err != nil {
				return toolResultMsg{
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error: " + err.Error(), IsError: true},
					toolName: tc.Name,
				}
			}
			if qr, ok := req.(*tool.QuestionRequest); ok {
				return QuestionRequestMsg{Request: qr}
			}
			if pr, ok := req.(*tool.PlanRequest); ok {
				return PlanRequestMsg{Request: pr}
			}
		}

		if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
			req, err := pat.PreparePermission(ctx, params, cwd)
			if err != nil {
				return toolResultMsg{
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error: " + err.Error(), IsError: true},
					toolName: tc.Name,
				}
			}
			return PermissionRequestMsg{Request: req}
		}

		result := tool.Execute(ctx, tc.Name, params, cwd)

		if tc.Name == "TodoWrite" && result.Success && len(result.TodoItems) > 0 {
			return todoResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
				toolName: tc.Name,
				todos:    result.TodoItems,
			}
		}

		return toolResultMsg{
			result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
			toolName: tc.Name,
		}
	}
}

func executeApprovedTool(toolCalls []provider.ToolCall, idx int, cwd string) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Internal error: unknown tool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		pat, ok := t.(tool.PermissionAwareTool)
		if !ok {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Internal error: tool does not implement PermissionAwareTool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		result := pat.ExecuteApproved(ctx, params, cwd)

		return toolResultMsg{
			result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
			toolName: tc.Name,
		}
	}
}

func executeInteractiveTool[T any](tc provider.ToolCall, response T, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Unknown tool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		it, ok := t.(tool.InteractiveTool)
		if !ok {
			return toolResultMsg{
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Tool is not interactive: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		result := it.ExecuteWithResponse(ctx, params, response, cwd)

		return toolResultMsg{
			result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
			toolName: tc.Name,
		}
	}
}

func parseToolInput(input string) (map[string]any, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return nil, err
	}
	return params, nil
}

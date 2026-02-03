package tui

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

type (
	startToolExecutionMsg struct {
		toolCalls []provider.ToolCall
	}
	allToolsCompletedMsg struct{}
	toolResultMsg        struct {
		index    int
		result   provider.ToolResult
		toolName string
	}
)

func (m model) executeTools(toolCalls []provider.ToolCall) tea.Cmd {
	return func() tea.Msg {
		return startToolExecutionMsg{toolCalls: toolCalls}
	}
}

// executeToolsParallel executes multiple tools in parallel and returns a batch command
func executeToolsParallel(toolCalls []provider.ToolCall, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	if len(toolCalls) == 0 {
		return func() tea.Msg {
			return allToolsCompletedMsg{}
		}
	}

	// For a single tool, use the existing sequential logic for simplicity
	// This ensures permission prompts work correctly
	if len(toolCalls) == 1 {
		return processNextTool(toolCalls, 0, cwd, settings, sessionPerms)
	}

	// Check if any tool requires permission - if so, process sequentially
	for _, tc := range toolCalls {
		params, err := parseToolInput(tc.Input)
		if err != nil {
			continue
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			continue
		}

		// Check if tool requires permission
		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			if permResult == config.PermissionAsk {
				// Has a tool that requires permission - use sequential processing
				return processNextTool(toolCalls, 0, cwd, settings, sessionPerms)
			}
		}

		// Check if it's a permission-aware tool
		if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
			// Use sequential processing for permission-aware tools
			return processNextTool(toolCalls, 0, cwd, settings, sessionPerms)
		}

		// Check if it's an interactive tool
		if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
			// Use sequential processing for interactive tools
			return processNextTool(toolCalls, 0, cwd, settings, sessionPerms)
		}
	}

	// All tools can run in parallel - execute them all at once
	var cmds []tea.Cmd
	for i, tc := range toolCalls {
		idx := i
		tcCopy := tc
		cmds = append(cmds, executeToolAsync(tcCopy, idx, cwd, settings, sessionPerms))
	}

	return tea.Batch(cmds...)
}

// executeToolAsync executes a single tool asynchronously and returns its result
func executeToolAsync(tc provider.ToolCall, index int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return toolResultMsg{
				index:    index,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		// Check if tool exists
		if _, ok := tool.Get(tc.Name); !ok {
			return toolResultMsg{
				index:    index,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Unknown tool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		// Check permission - if auto-allowed or denied, handle here
		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			switch permResult {
			case config.PermissionAllow:
				result := tool.Execute(ctx, tc.Name, params, cwd)
				return toolResultMsg{
					index:    index,
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
					toolName: tc.Name,
				}
			case config.PermissionDeny:
				return toolResultMsg{
					index:    index,
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Permission denied by settings", IsError: true},
					toolName: tc.Name,
				}
			}
		}

		// Execute the tool
		result := tool.Execute(ctx, tc.Name, params, cwd)

		return toolResultMsg{
			index:    index,
			result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
			toolName: tc.Name,
		}
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
				index:    idx,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				index:    idx,
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
					index:    idx,
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: result.FormatForLLM(), IsError: !result.Success},
					toolName: tc.Name,
				}
			case config.PermissionDeny:
				return toolResultMsg{
					index:    idx,
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
					index:    idx,
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
			if epr, ok := req.(*tool.EnterPlanRequest); ok {
				return EnterPlanRequestMsg{Request: epr}
			}
		}

		if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
			req, err := pat.PreparePermission(ctx, params, cwd)
			if err != nil {
				return toolResultMsg{
					index:    idx,
					result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error: " + err.Error(), IsError: true},
					toolName: tc.Name,
				}
			}
			return PermissionRequestMsg{Request: req}
		}

		result := tool.Execute(ctx, tc.Name, params, cwd)

		return toolResultMsg{
			index:    idx,
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
				index:    idx,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Error parsing tool input: " + err.Error(), IsError: true},
				toolName: tc.Name,
			}
		}

		// For Task tool, set up progress callback
		if tc.Name == "Task" {
			params["_onProgress"] = tool.ProgressFunc(func(msg string) {
				SendTaskProgress(msg)
			})
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				index:    idx,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Internal error: unknown tool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		pat, ok := t.(tool.PermissionAwareTool)
		if !ok {
			return toolResultMsg{
				index:    idx,
				result:   provider.ToolResult{ToolCallID: tc.ID, Content: "Internal error: tool does not implement PermissionAwareTool: " + tc.Name, IsError: true},
				toolName: tc.Name,
			}
		}

		result := pat.ExecuteApproved(ctx, params, cwd)

		return toolResultMsg{
			index:    idx,
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

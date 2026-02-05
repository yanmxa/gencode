package tui

import (
	"context"
	"encoding/json"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
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

// newToolResult creates a toolResultMsg with the given parameters
func newToolResult(tc provider.ToolCall, index int, content string, isError bool) toolResultMsg {
	return toolResultMsg{
		index:    index,
		result:   provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		toolName: tc.Name,
	}
}

// newToolResultFromOutput creates a toolResultMsg from a ui.ToolResult
func newToolResultFromOutput(tc provider.ToolCall, index int, output ui.ToolResult) toolResultMsg {
	return toolResultMsg{
		index:    index,
		result:   provider.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success},
		toolName: tc.Name,
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
			return newToolResult(tc, index, "Error parsing tool input: "+err.Error(), true)
		}

		if _, ok := tool.Get(tc.Name); !ok {
			return newToolResult(tc, index, "Unknown tool: "+tc.Name, true)
		}

		// Check permission - if auto-allowed or denied, handle here
		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			switch permResult {
			case config.PermissionAllow:
				start := time.Now()
				result := tool.Execute(ctx, tc.Name, params, cwd)
				log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
				return newToolResultFromOutput(tc, index, result)
			case config.PermissionDeny:
				return newToolResult(tc, index, "Permission denied by settings", true)
			}
		}

		start := time.Now()
		result := tool.Execute(ctx, tc.Name, params, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newToolResultFromOutput(tc, index, result)
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
			return newToolResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newToolResult(tc, idx, "Unknown tool: "+tc.Name, true)
		}

		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			switch permResult {
			case config.PermissionAllow:
				start := time.Now()
				result := tool.Execute(ctx, tc.Name, params, cwd)
				log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
				return newToolResultFromOutput(tc, idx, result)
			case config.PermissionDeny:
				return newToolResult(tc, idx, "Permission denied by settings", true)
			case config.PermissionAsk:
				// Fall through
			}
		}

		if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
			req, err := it.PrepareInteraction(ctx, params, cwd)
			if err != nil {
				return newToolResult(tc, idx, "Error: "+err.Error(), true)
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
				return newToolResult(tc, idx, "Error: "+err.Error(), true)
			}
			return PermissionRequestMsg{Request: req}
		}

		start := time.Now()
		result := tool.Execute(ctx, tc.Name, params, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newToolResultFromOutput(tc, idx, result)
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
			return newToolResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		// For Task tool, set up progress callback
		if tc.Name == "Task" {
			params["_onProgress"] = tool.ProgressFunc(func(msg string) {
				SendTaskProgress(msg)
			})
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newToolResult(tc, idx, "Internal error: unknown tool: "+tc.Name, true)
		}

		pat, ok := t.(tool.PermissionAwareTool)
		if !ok {
			return newToolResult(tc, idx, "Internal error: tool does not implement PermissionAwareTool: "+tc.Name, true)
		}

		start := time.Now()
		result := pat.ExecuteApproved(ctx, params, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newToolResultFromOutput(tc, idx, result)
	}
}

func executeInteractiveTool[T any](tc provider.ToolCall, response T, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newToolResult(tc, 0, "Error parsing tool input: "+err.Error(), true)
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newToolResult(tc, 0, "Unknown tool: "+tc.Name, true)
		}

		it, ok := t.(tool.InteractiveTool)
		if !ok {
			return newToolResult(tc, 0, "Tool is not interactive: "+tc.Name, true)
		}

		start := time.Now()
		result := it.ExecuteWithResponse(ctx, params, response, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newToolResultFromOutput(tc, 0, result)
	}
}

func parseToolInput(input string) (map[string]any, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return nil, err
	}
	return params, nil
}

// encodeToolInput converts params back to JSON string for tool input
func encodeToolInput(params map[string]any) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

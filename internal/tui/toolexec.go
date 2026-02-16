package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type (
	startToolExecutionMsg struct {
		toolCalls []message.ToolCall
	}
	allToolsCompletedMsg struct{}
	toolResultMsg        struct {
		index    int
		result   message.ToolResult
		toolName string
	}
)

func (m model) executeTools(toolCalls []message.ToolCall) tea.Cmd {
	return func() tea.Msg {
		return startToolExecutionMsg{toolCalls: toolCalls}
	}
}

// newToolResult creates a toolResultMsg with the given parameters
func newToolResult(tc message.ToolCall, index int, content string, isError bool) toolResultMsg {
	return toolResultMsg{
		index:    index,
		result:   message.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		toolName: tc.Name,
	}
}

// newToolResultFromOutput creates a toolResultMsg from a ui.ToolResult
func newToolResultFromOutput(tc message.ToolCall, index int, output ui.ToolResult) toolResultMsg {
	return toolResultMsg{
		index:    index,
		result:   message.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success},
		toolName: tc.Name,
	}
}

// executeToolsParallel executes multiple tools in parallel and returns a batch command
func executeToolsParallel(toolCalls []message.ToolCall, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
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

	// Check if any tool requires user interaction - if so, process sequentially
	for _, tc := range toolCalls {
		if requiresUserInteraction(tc, settings, sessionPerms) {
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

// executeToolAsync executes a single tool asynchronously and returns its result.
func executeToolAsync(tc message.ToolCall, index int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newToolResult(tc, index, "Error parsing tool input: "+err.Error(), true)
		}

		// Check if this is an MCP tool
		if mcp.IsMCPTool(tc.Name) {
			return executeAndLog(tc, index, func() ui.ToolResult {
				return executeMCPTool(ctx, tc, params)
			})
		}

		if _, ok := tool.Get(tc.Name); !ok {
			return newToolResult(tc, index, "Unknown tool: "+tc.Name, true)
		}

		// Check permission
		if settings != nil {
			switch settings.CheckPermission(tc.Name, params, sessionPerms) {
			case config.PermissionDeny:
				return newToolResult(tc, index, "Permission denied by settings", true)
			case config.PermissionAllow:
				// Fall through to execute
			}
		}

		return executeAndLog(tc, index, func() ui.ToolResult {
			return tool.Execute(ctx, tc.Name, params, cwd)
		})
	}
}

// executeAndLog runs a tool function, logs execution time, and returns the result.
func executeAndLog(tc message.ToolCall, index int, fn func() ui.ToolResult) toolResultMsg {
	start := time.Now()
	result := fn()
	log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
	return newToolResultFromOutput(tc, index, result)
}

func processNextTool(toolCalls []message.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	if idx >= len(toolCalls) {
		return func() tea.Msg { return allToolsCompletedMsg{} }
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newToolResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		// MCP tools execute directly
		if mcp.IsMCPTool(tc.Name) {
			return executeAndLog(tc, idx, func() ui.ToolResult {
				return executeMCPTool(ctx, tc, params)
			})
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newToolResult(tc, idx, "Unknown tool: "+tc.Name, true)
		}

		// Check permissions from settings
		if settings != nil {
			switch settings.CheckPermission(tc.Name, params, sessionPerms) {
			case config.PermissionAllow:
				return executeAndLog(tc, idx, func() ui.ToolResult {
					return tool.Execute(ctx, tc.Name, params, cwd)
				})
			case config.PermissionDeny:
				return newToolResult(tc, idx, "Permission denied by settings", true)
			}
		}

		// Check for interactive tool prompts
		if msg := checkInteractiveTool(ctx, t, tc, idx, params, cwd); msg != nil {
			return msg
		}

		// Check for permission-aware tool prompts
		if msg := checkPermissionTool(ctx, t, tc, idx, params, cwd); msg != nil {
			return msg
		}

		return executeAndLog(tc, idx, func() ui.ToolResult {
			return tool.Execute(ctx, tc.Name, params, cwd)
		})
	}
}

// checkInteractiveTool checks if the tool requires interaction and returns the appropriate message.
func checkInteractiveTool(ctx context.Context, t tool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	it, ok := t.(tool.InteractiveTool)
	if !ok || !it.RequiresInteraction() {
		return nil
	}

	req, err := it.PrepareInteraction(ctx, params, cwd)
	if err != nil {
		return newToolResult(tc, idx, "Error: "+err.Error(), true)
	}

	switch r := req.(type) {
	case *tool.QuestionRequest:
		return QuestionRequestMsg{Request: r}
	case *tool.PlanRequest:
		return PlanRequestMsg{Request: r}
	case *tool.EnterPlanRequest:
		return EnterPlanRequestMsg{Request: r}
	}
	return nil
}

// checkPermissionTool checks if the tool requires permission and returns the appropriate message.
func checkPermissionTool(ctx context.Context, t tool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	pat, ok := t.(tool.PermissionAwareTool)
	if !ok || !pat.RequiresPermission() {
		return nil
	}

	req, err := pat.PreparePermission(ctx, params, cwd)
	if err != nil {
		return newToolResult(tc, idx, "Error: "+err.Error(), true)
	}
	return PermissionRequestMsg{Request: req}
}

func executeApprovedTool(toolCalls []message.ToolCall, idx int, cwd string) tea.Cmd {
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

func executeInteractiveTool[T any](tc message.ToolCall, response T, cwd string) tea.Cmd {
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
	return message.ParseToolInput(input)
}

// requiresUserInteraction checks if a tool call requires user interaction (permission or interactive prompt)
func requiresUserInteraction(tc message.ToolCall, settings *config.Settings, sessionPerms *config.SessionPermissions) bool {
	params, err := parseToolInput(tc.Input)
	if err != nil {
		return true // Assume interaction required on parse error
	}

	t, ok := tool.Get(tc.Name)
	if !ok {
		return true // Unknown tool, assume interaction required
	}

	// Check settings permission
	if settings != nil {
		if settings.CheckPermission(tc.Name, params, sessionPerms) == config.PermissionAsk {
			return true
		}
	}

	// Check permission-aware tool
	if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
		return true
	}

	// Check interactive tool
	if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
		return true
	}

	return false
}

// executeMCPTool executes an MCP tool and returns the result
func executeMCPTool(ctx context.Context, tc message.ToolCall, params map[string]any) ui.ToolResult {
	if mcp.DefaultRegistry == nil {
		return ui.NewErrorResult(tc.Name, "MCP registry not initialized")
	}

	result, err := mcp.DefaultRegistry.CallTool(ctx, tc.Name, params)
	if err != nil {
		return ui.NewErrorResult(tc.Name, err.Error())
	}

	return ui.ToolResult{
		Success:  !result.IsError,
		Output:   extractMCPContent(result.Content),
		Metadata: ui.ResultMetadata{Title: tc.Name, Icon: "🔌"},
	}
}

// extractMCPContent extracts text content from MCP tool result
func extractMCPContent(contents []mcp.ToolResultContent) string {
	var parts []string
	for _, c := range contents {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}


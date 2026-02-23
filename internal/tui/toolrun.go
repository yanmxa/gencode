package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
	"github.com/yanmxa/gencode/internal/tui/progress"
)

type StartMsg struct {
	ToolCalls []message.ToolCall
}

type CompletedMsg struct{}

type ResultMsg struct {
	Index    int
	Result   message.ToolResult
	ToolName string
}

func newResult(tc message.ToolCall, index int, content string, isError bool) ResultMsg {
	return ResultMsg{
		Index:    index,
		Result:   message.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		ToolName: tc.Name,
	}
}

func newResultFromOutput(tc message.ToolCall, index int, output ui.ToolResult) ResultMsg {
	return ResultMsg{
		Index:    index,
		Result:   message.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success},
		ToolName: tc.Name,
	}
}

func ExecuteParallel(toolCalls []message.ToolCall, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool) tea.Cmd {
	if len(toolCalls) == 0 {
		return func() tea.Msg {
			return CompletedMsg{}
		}
	}

	if len(toolCalls) == 1 {
		if !RequiresUserInteraction(toolCalls[0], settings, sessionPerms, planMode) {
			return executeToolAsync(toolCalls[0], 0, cwd, settings, sessionPerms)
		}
		return ProcessNext(toolCalls, 0, cwd, settings, sessionPerms)
	}

	for _, tc := range toolCalls {
		if RequiresUserInteraction(tc, settings, sessionPerms, planMode) {
			return ProcessNext(toolCalls, 0, cwd, settings, sessionPerms)
		}
	}

	var cmds []tea.Cmd
	for i, tc := range toolCalls {
		idx := i
		tcCopy := tc
		cmds = append(cmds, executeToolAsync(tcCopy, idx, cwd, settings, sessionPerms))
	}

	return tea.Batch(cmds...)
}

func executeToolAsync(tc message.ToolCall, index int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, index, "Error parsing tool input: "+err.Error(), true)
		}

		if tc.Name == "Task" {
			idx := index
			params["_onProgress"] = tool.ProgressFunc(func(msg string) {
				progress.SendForAgent(idx, msg)
			})
		}

		if mcp.IsMCPTool(tc.Name) {
			return executeAndLog(tc, index, func() ui.ToolResult {
				return executeMCPTool(ctx, tc, params)
			})
		}

		if _, ok := tool.Get(tc.Name); !ok {
			return newResult(tc, index, "Unknown tool: "+tc.Name, true)
		}

		if msg := checkPermission(tc, index, params, settings, sessionPerms); msg != nil {
			return msg
		}

		return executeAndLog(tc, index, func() ui.ToolResult {
			return tool.Execute(ctx, tc.Name, params, cwd)
		})
	}
}

func checkPermission(tc message.ToolCall, index int, params map[string]any, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Msg {
	if settings == nil {
		return nil
	}
	switch settings.CheckPermission(tc.Name, params, sessionPerms) {
	case config.PermissionDeny:
		return newResult(tc, index, "Permission denied by settings", true)
	default:
		return nil
	}
}

func executeAndLog(tc message.ToolCall, index int, fn func() ui.ToolResult) ResultMsg {
	start := time.Now()
	result := fn()
	log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
	return newResultFromOutput(tc, index, result)
}

func ProcessNext(toolCalls []message.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	if idx >= len(toolCalls) {
		return func() tea.Msg { return CompletedMsg{} }
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		if mcp.IsMCPTool(tc.Name) {
			return executeAndLog(tc, idx, func() ui.ToolResult {
				return executeMCPTool(ctx, tc, params)
			})
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newResult(tc, idx, "Unknown tool: "+tc.Name, true)
		}

		if settings != nil {
			switch settings.CheckPermission(tc.Name, params, sessionPerms) {
			case config.PermissionAllow:
				return executeAndLog(tc, idx, func() ui.ToolResult {
					return tool.Execute(ctx, tc.Name, params, cwd)
				})
			case config.PermissionDeny:
				return newResult(tc, idx, "Permission denied by settings", true)
			}
		}

		if msg := checkInteractiveTool(ctx, t, tc, idx, params, cwd); msg != nil {
			return msg
		}

		if msg := checkPermissionTool(ctx, t, tc, idx, params, cwd); msg != nil {
			return msg
		}

		return executeAndLog(tc, idx, func() ui.ToolResult {
			return tool.Execute(ctx, tc.Name, params, cwd)
		})
	}
}

func checkInteractiveTool(ctx context.Context, t tool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	it, ok := t.(tool.InteractiveTool)
	if !ok || !it.RequiresInteraction() {
		return nil
	}

	req, err := it.PrepareInteraction(ctx, params, cwd)
	if err != nil {
		return newResult(tc, idx, "Error: "+err.Error(), true)
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

func checkPermissionTool(ctx context.Context, t tool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	pat, ok := t.(tool.PermissionAwareTool)
	if !ok || !pat.RequiresPermission() {
		return nil
	}

	req, err := pat.PreparePermission(ctx, params, cwd)
	if err != nil {
		return newResult(tc, idx, "Error: "+err.Error(), true)
	}
	return PermissionRequestMsg{Request: req}
}

func ExecuteApproved(toolCalls []message.ToolCall, idx int, cwd string) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		if tc.Name == "Task" {
			agentIdx := idx
			params["_onProgress"] = tool.ProgressFunc(func(msg string) {
				progress.SendForAgent(agentIdx, msg)
			})
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newResult(tc, idx, "Internal error: unknown tool: "+tc.Name, true)
		}

		pat, ok := t.(tool.PermissionAwareTool)
		if !ok {
			return newResult(tc, idx, "Internal error: tool does not implement PermissionAwareTool: "+tc.Name, true)
		}

		start := time.Now()
		result := pat.ExecuteApproved(ctx, params, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newResultFromOutput(tc, idx, result)
	}
}

func ExecuteInteractive[T any](tc message.ToolCall, response T, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, 0, "Error parsing tool input: "+err.Error(), true)
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return newResult(tc, 0, "Unknown tool: "+tc.Name, true)
		}

		it, ok := t.(tool.InteractiveTool)
		if !ok {
			return newResult(tc, 0, "Tool is not interactive: "+tc.Name, true)
		}

		start := time.Now()
		result := it.ExecuteWithResponse(ctx, params, response, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newResultFromOutput(tc, 0, result)
	}
}

func parseToolInput(input string) (map[string]any, error) {
	return message.ParseToolInput(input)
}

func RequiresUserInteraction(tc message.ToolCall, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool) bool {
	if planMode && tc.Name == "Task" {
		return false
	}

	params, err := parseToolInput(tc.Input)
	if err != nil {
		return true
	}

	t, ok := tool.Get(tc.Name)
	if !ok {
		return true
	}

	if settings != nil {
		if settings.CheckPermission(tc.Name, params, sessionPerms) == config.PermissionAsk {
			return true
		}
	}

	if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
		return true
	}

	if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
		return true
	}

	return false
}

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

func extractMCPContent(contents []mcp.ToolResultContent) string {
	var parts []string
	for _, c := range contents {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

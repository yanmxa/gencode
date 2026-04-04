// Tool execution: parallel/sequential dispatch, permission checks, MCP tool support.
package tool

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

type defaultMCPExecutor struct{}

func (defaultMCPExecutor) IsMCPTool(name string) bool {
	return mcp.IsMCPTool(name)
}

func (defaultMCPExecutor) ExecuteMCP(ctx context.Context, name string, params map[string]any) (ui.ToolResult, error) {
	if mcp.DefaultRegistry == nil {
		return ui.NewErrorResult(name, "MCP registry not initialized"), nil
	}

	result, err := mcp.DefaultRegistry.CallTool(ctx, name, params)
	if err != nil {
		return ui.NewErrorResult(name, err.Error()), nil
	}

	return ui.ToolResult{
		Success:  !result.IsError,
		Output:   extractMCPContent(result.Content),
		Metadata: ui.ResultMetadata{Title: name, Icon: "🔌"},
	}, nil
}

// ExecStartMsg signals the parent to begin executing tool calls.
type ExecStartMsg struct {
	ToolCalls []message.ToolCall
}

// ExecDoneMsg signals that all tool calls have completed.
type ExecDoneMsg struct{}

// ExecResultMsg carries the result of a single tool execution.
type ExecResultMsg struct {
	Index    int
	Result   message.ToolResult
	ToolName string
}

func newResult(tc message.ToolCall, index int, content string, isError bool) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   message.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		ToolName: tc.Name,
	}
}

func newResultFromOutput(tc message.ToolCall, index int, output ui.ToolResult) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   message.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success, HookResponse: output.HookResponse},
		ToolName: tc.Name,
	}
}

// ExecuteParallel dispatches tool calls in parallel when possible, sequentially otherwise.
// hookAllowed contains tool call IDs that were pre-approved by hooks (may be nil).
func ExecuteParallel(ctx context.Context, hub *progress.Hub, toolCalls []message.ToolCall, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool, hookAllowed map[string]bool) tea.Cmd {
	if len(toolCalls) == 0 {
		return func() tea.Msg {
			return ExecDoneMsg{}
		}
	}

	if len(toolCalls) == 1 {
		if !RequiresUserInteraction(toolCalls[0], settings, sessionPerms, planMode, hookAllowed) {
			return executeToolAsync(ctx, hub, toolCalls[0], 0, cwd, settings, sessionPerms)
		}
		return ProcessNext(ctx, hub, toolCalls, 0, cwd, settings, sessionPerms)
	}

	for _, tc := range toolCalls {
		if RequiresUserInteraction(tc, settings, sessionPerms, planMode, hookAllowed) {
			return ProcessNext(ctx, hub, toolCalls, 0, cwd, settings, sessionPerms)
		}
	}

	var cmds []tea.Cmd
	for i, tc := range toolCalls {
		idx := i
		tcCopy := tc
		cmds = append(cmds, executeToolAsync(ctx, hub, tcCopy, idx, cwd, settings, sessionPerms))
	}

	return tea.Batch(cmds...)
}

func executeToolAsync(ctx context.Context, hub *progress.Hub, tc message.ToolCall, index int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	return func() tea.Msg {
		ctx = executionContext(ctx)

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, index, "Error parsing tool input: "+err.Error(), true)
		}

		if tc.Name == coretool.ToolAgent {
			idx := index
			params["_onProgress"] = coretool.ProgressFunc(func(msg string) {
				progressHub(hub).SendForAgent(idx, msg)
			})
		}

		if _, ok := coretool.Get(tc.Name); !ok && !mcp.IsMCPTool(tc.Name) {
			return newResult(tc, index, "Unknown tool: "+tc.Name, true)
		}

		if msg := checkPermission(tc, index, params, settings, sessionPerms); msg != nil {
			return msg
		}

		return executeAndLog(tc, index, func() ui.ToolResult {
			result, err := coretool.ExecutePreparedTool(ctx, tc, params, cwd, false, defaultMCPExecutor{})
			if err != nil {
				return ui.NewErrorResult(tc.Name, err.Error())
			}
			return result
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

func executeAndLog(tc message.ToolCall, index int, fn func() ui.ToolResult) ExecResultMsg {
	start := time.Now()
	result := fn()
	log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
	return newResultFromOutput(tc, index, result)
}

// ProcessNext executes the next tool call in sequence.
func ProcessNext(ctx context.Context, hub *progress.Hub, toolCalls []message.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	if idx >= len(toolCalls) {
		return func() tea.Msg { return ExecDoneMsg{} }
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx = executionContext(ctx)

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		t, ok := coretool.Get(tc.Name)
		if !ok && !mcp.IsMCPTool(tc.Name) {
			return newResult(tc, idx, "Unknown tool: "+tc.Name, true)
		}

		if settings != nil {
			switch settings.CheckPermission(tc.Name, params, sessionPerms) {
			case config.PermissionAllow:
				return executeAndLog(tc, idx, func() ui.ToolResult {
					result, err := coretool.ExecutePreparedTool(ctx, tc, params, cwd, false, defaultMCPExecutor{})
					if err != nil {
						return ui.NewErrorResult(tc.Name, err.Error())
					}
					return result
				})
			case config.PermissionDeny:
				return newResult(tc, idx, "Permission denied by settings", true)
			}
		}

		if ok {
			if msg := checkInteractiveTool(ctx, t, tc, idx, params, cwd); msg != nil {
				return msg
			}

			if msg := checkPermissionTool(ctx, t, tc, idx, params, cwd); msg != nil {
				return msg
			}
		}

		return executeAndLog(tc, idx, func() ui.ToolResult {
			result, err := coretool.ExecutePreparedTool(ctx, tc, params, cwd, false, defaultMCPExecutor{})
			if err != nil {
				return ui.NewErrorResult(tc.Name, err.Error())
			}
			return result
		})
	}
}

func checkInteractiveTool(ctx context.Context, t coretool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	it, ok := t.(coretool.InteractiveTool)
	if !ok || !it.RequiresInteraction() {
		return nil
	}

	req, err := it.PrepareInteraction(ctx, params, cwd)
	if err != nil {
		return newResult(tc, idx, "Error: "+err.Error(), true)
	}

	switch r := req.(type) {
	case *coretool.QuestionRequest:
		return appmode.QuestionRequestMsg{Request: r}
	case *coretool.PlanRequest:
		return appmode.PlanRequestMsg{Request: r}
	case *coretool.EnterPlanRequest:
		return appmode.EnterPlanRequestMsg{Request: r}
	}
	return nil
}

func checkPermissionTool(ctx context.Context, t coretool.Tool, tc message.ToolCall, idx int, params map[string]any, cwd string) tea.Msg {
	pat, ok := t.(coretool.PermissionAwareTool)
	if !ok || !pat.RequiresPermission() {
		return nil
	}

	req, err := pat.PreparePermission(ctx, params, cwd)
	if err != nil {
		return newResult(tc, idx, "Error: "+err.Error(), true)
	}
	return appapproval.RequestMsg{Request: req}
}

// ExecuteApproved executes a tool that has been approved by the user.
func ExecuteApproved(ctx context.Context, hub *progress.Hub, toolCalls []message.ToolCall, idx int, cwd string) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx = executionContext(ctx)

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, idx, "Error parsing tool input: "+err.Error(), true)
		}

		if tc.Name == coretool.ToolAgent {
			agentIdx := idx
			params["_onProgress"] = coretool.ProgressFunc(func(msg string) {
				progressHub(hub).SendForAgent(agentIdx, msg)
			})
		}

		start := time.Now()
		result, err := coretool.ExecutePreparedTool(ctx, tc, params, cwd, true, defaultMCPExecutor{})
		if err != nil {
			if mcp.IsMCPTool(tc.Name) {
				return newResult(tc, idx, "Internal error: "+err.Error(), true)
			}
			return newResult(tc, idx, "Internal error: unknown tool: "+tc.Name, true)
		}
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newResultFromOutput(tc, idx, result)
	}
}

// ExecuteInteractive executes a tool with an interactive response.
func ExecuteInteractive[T any](ctx context.Context, tc message.ToolCall, response T, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx = executionContext(ctx)

		params, err := parseToolInput(tc.Input)
		if err != nil {
			return newResult(tc, 0, "Error parsing tool input: "+err.Error(), true)
		}

		t, ok := coretool.Get(tc.Name)
		if !ok {
			return newResult(tc, 0, "Unknown tool: "+tc.Name, true)
		}

		it, ok := t.(coretool.InteractiveTool)
		if !ok {
			return newResult(tc, 0, "Tool is not interactive: "+tc.Name, true)
		}

		start := time.Now()
		result := it.ExecuteWithResponse(ctx, params, response, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newResultFromOutput(tc, 0, result)
	}
}

// RequiresUserInteraction checks if a tool call needs user approval.
// hookAllowed contains tool call IDs that were pre-approved by hooks (may be nil).
func RequiresUserInteraction(tc message.ToolCall, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool, hookAllowed map[string]bool) bool {
	if planMode && tc.Name == coretool.ToolAgent {
		return false
	}

	params, err := parseToolInput(tc.Input)
	if err != nil {
		return true
	}

	// If hook pre-approved this tool call, validate against safety invariant
	if hookAllowed[tc.ID] && settings != nil {
		if settings.ResolveHookAllow(tc.Name, params, sessionPerms) {
			return false // hook allow is valid, skip interaction
		}
		// Safety invariant denied the hook allow — fall through to normal check
	}

	t, ok := coretool.Get(tc.Name)
	if !ok {
		return true
	}

	// Check settings + session permissions first.
	// If explicitly allowed (e.g., "allow all agents this session"),
	// skip the tool's built-in permission requirement.
	if settings != nil {
		perm := settings.CheckPermission(tc.Name, params, sessionPerms)
		switch perm {
		case config.PermissionAllow:
			return false // session/settings explicitly allow — no interaction needed
		case config.PermissionAsk:
			return true
		}
	}

	if pat, ok := t.(coretool.PermissionAwareTool); ok && pat.RequiresPermission() {
		return true
	}

	if it, ok := t.(coretool.InteractiveTool); ok && it.RequiresInteraction() {
		return true
	}

	return false
}

func parseToolInput(input string) (map[string]any, error) {
	return message.ParseToolInput(input)
}

func executionContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func progressHub(hub *progress.Hub) *progress.Hub {
	if hub != nil {
		return hub
	}
	return progress.NewHub(100)
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

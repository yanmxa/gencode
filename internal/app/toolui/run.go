// Tool execution: parallel/sequential dispatch, permission checks, MCP tool support.
package toolui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/core"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
	"github.com/yanmxa/gencode/internal/app/progress"
)

type defaultMCPExecutor struct{}

func (defaultMCPExecutor) IsMCPTool(name string) bool {
	return mcp.IsMCPTool(name)
}

func (defaultMCPExecutor) ExecuteMCP(ctx context.Context, name string, params map[string]any) (toolresult.ToolResult, error) {
	if mcp.DefaultRegistry == nil {
		return toolresult.NewErrorResult(name, "MCP registry not initialized"), nil
	}

	result, err := mcp.DefaultRegistry.CallTool(ctx, name, params)
	if err != nil {
		return toolresult.NewErrorResult(name, err.Error()), nil
	}

	return toolresult.ToolResult{
		Success:  !result.IsError,
		Output:   mcp.ExtractContent(result.Content),
		Metadata: toolresult.ResultMetadata{Title: name, Icon: "🔌"},
	}, nil
}

// ExecDoneMsg signals that all tool calls have completed.
type ExecDoneMsg struct{}

// ExecResultMsg carries the result of a single tool execution.
type ExecResultMsg struct {
	Index    int
	Result   core.ToolResult
	ToolName string
}

func newResult(tc core.ToolCall, index int, content string, isError bool) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   core.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		ToolName: tc.Name,
	}
}

func newResultFromOutput(tc core.ToolCall, index int, output toolresult.ToolResult) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   core.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success, HookResponse: output.HookResponse},
		ToolName: tc.Name,
	}
}

// ExecuteParallel dispatches tool calls in parallel when possible, sequentially otherwise.
// hookAllowed contains tool call IDs that were pre-approved by hooks (may be nil).
// hookForceAsk contains tool call IDs forced to prompt by PreToolUse "ask" decision (may be nil).
func ExecuteParallel(ctx context.Context, hub *progress.Hub, toolCalls []core.ToolCall, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool, hookAllowed map[string]bool, hookForceAsk map[string]bool) tea.Cmd {
	if len(toolCalls) == 0 {
		return func() tea.Msg {
			return ExecDoneMsg{}
		}
	}

	if len(toolCalls) == 1 {
		if !RequiresUserInteraction(toolCalls[0], settings, sessionPerms, planMode, hookAllowed, hookForceAsk) {
			return executeToolAsync(ctx, hub, toolCalls[0], 0, cwd, settings, sessionPerms)
		}
		return ProcessNext(ctx, hub, toolCalls, 0, cwd, settings, sessionPerms, hookAllowed, hookForceAsk)
	}

	for _, tc := range toolCalls {
		if RequiresUserInteraction(tc, settings, sessionPerms, planMode, hookAllowed, hookForceAsk) {
			return ProcessNext(ctx, hub, toolCalls, 0, cwd, settings, sessionPerms, hookAllowed, hookForceAsk)
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

func executeToolAsync(ctx context.Context, hub *progress.Hub, tc core.ToolCall, index int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	return func() tea.Msg {
		ctx = executionContext(ctx)

		prepared, err := coretool.PrepareToolCall(tc, defaultMCPExecutor{})
		if err != nil {
			return newResult(tc, index, formatPrepareError(err), true)
		}

		attachAgentCallbacks(ctx, hub, index, prepared)

		if msg := checkInteractiveTool(ctx, prepared, index, cwd); msg != nil {
			return msg
		}

		if msg := checkPermission(prepared, index, settings, sessionPerms); msg != nil {
			return msg
		}

		return executeAndLog(prepared, index, func() toolresult.ToolResult {
			result, err := prepared.Execute(ctx, cwd, false, defaultMCPExecutor{})
			if err != nil {
				return toolresult.NewErrorResult(prepared.Call.Name, err.Error())
			}
			return result
		})
	}
}

func checkPermission(prepared *coretool.PreparedToolCall, index int, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Msg {
	if settings == nil {
		return nil
	}
	switch settings.CheckPermission(prepared.Call.Name, prepared.Params, sessionPerms) {
	case config.Deny:
		return newResult(prepared.Call, index, "Permission denied by settings", true)
	default:
		return nil
	}
}

func executeAndLog(prepared *coretool.PreparedToolCall, index int, fn func() toolresult.ToolResult) ExecResultMsg {
	start := time.Now()
	result := fn()
	log.LogTool(prepared.Call.Name, prepared.Call.ID, time.Since(start).Milliseconds(), result.Success)
	return newResultFromOutput(prepared.Call, index, result)
}

// ProcessNext executes the next tool call in sequence.
// hookAllowed/hookForceAsk are maps of pre-approved or force-ask tool call IDs from hooks (may be nil).
func ProcessNext(ctx context.Context, hub *progress.Hub, toolCalls []core.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions, hookAllowed, hookForceAsk map[string]bool) tea.Cmd {
	if idx >= len(toolCalls) {
		return func() tea.Msg { return ExecDoneMsg{} }
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx = executionContext(ctx)

		prepared, err := coretool.PrepareToolCall(tc, defaultMCPExecutor{})
		if err != nil {
			return newResult(tc, idx, formatPrepareError(err), true)
		}

		if settings != nil {
			switch settings.CheckPermission(prepared.Call.Name, prepared.Params, sessionPerms) {
			case config.Deny:
				return newResult(prepared.Call, idx, "Permission denied by settings", true)
			}
		}

		if msg := checkInteractiveTool(ctx, prepared, idx, cwd); msg != nil {
			return msg
		}

		if msg := checkPermissionTool(ctx, prepared, idx, cwd); msg != nil {
			return msg
		}

		attachAgentCallbacks(ctx, hub, idx, prepared)

		return executeAndLog(prepared, idx, func() toolresult.ToolResult {
			result, err := prepared.Execute(ctx, cwd, false, defaultMCPExecutor{})
			if err != nil {
				return toolresult.NewErrorResult(prepared.Call.Name, err.Error())
			}
			return result
		})
	}
}

func checkInteractiveTool(ctx context.Context, prepared *coretool.PreparedToolCall, idx int, cwd string) tea.Msg {
	it, ok := prepared.Tool.(coretool.InteractiveTool)
	if !ok || !it.RequiresInteraction() {
		return nil
	}

	req, err := it.PrepareInteraction(ctx, prepared.Params, cwd)
	if err != nil {
		return newResult(prepared.Call, idx, "Error: "+err.Error(), true)
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

func checkPermissionTool(ctx context.Context, prepared *coretool.PreparedToolCall, idx int, cwd string) tea.Msg {
	pat, ok := prepared.Tool.(coretool.PermissionAwareTool)
	if !ok || !pat.RequiresPermission() {
		return nil
	}

	req, err := pat.PreparePermission(ctx, prepared.Params, cwd)
	if err != nil {
		return newResult(prepared.Call, idx, "Error: "+err.Error(), true)
	}
	return appapproval.RequestMsg{Request: req}
}

// ExecuteApproved executes a tool that has been approved by the user.
func ExecuteApproved(ctx context.Context, hub *progress.Hub, toolCalls []core.ToolCall, idx int, cwd string) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx = executionContext(ctx)

		prepared, err := coretool.PrepareToolCall(tc, defaultMCPExecutor{})
		if err != nil {
			return newResult(tc, idx, formatPrepareError(err), true)
		}

		attachAgentCallbacks(ctx, hub, idx, prepared)

		start := time.Now()
		result, err := prepared.Execute(ctx, cwd, true, defaultMCPExecutor{})
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

func attachAgentCallbacks(ctx context.Context, hub *progress.Hub, idx int, prepared *coretool.PreparedToolCall) {
	if !coretool.IsAgentToolName(prepared.Call.Name) {
		return
	}

	prepared.Params["_onProgress"] = coretool.ProgressFunc(func(msg string) {
		sendAgentProgress(hub, idx, msg)
	})
	prepared.Params["_onQuestion"] = coretool.AskQuestionFunc(func(qctx context.Context, req *coretool.QuestionRequest) (*coretool.QuestionResponse, error) {
		if qctx == nil {
			qctx = ctx
		}
		return askAgentQuestion(qctx, hub, idx, req)
	})

	// Inject parent messages getter for fork support (from context)
	if getter := coretool.GetMessagesGetter(ctx); getter != nil {
		prepared.Params["_messagesGetter"] = getter
	}
}

func askAgentQuestion(ctx context.Context, hub *progress.Hub, idx int, req *coretool.QuestionRequest) (*coretool.QuestionResponse, error) {
	if hub == nil {
		return nil, context.Canceled
	}
	return hub.Ask(ctx, idx, req)
}

// ExecuteInteractive executes a tool with an interactive response.
func ExecuteInteractive[T any](ctx context.Context, tc core.ToolCall, response T, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx = executionContext(ctx)

		prepared, err := coretool.PrepareToolCall(tc, defaultMCPExecutor{})
		if err != nil {
			return newResult(tc, 0, formatPrepareError(err), true)
		}

		it, ok := prepared.Tool.(coretool.InteractiveTool)
		if !ok {
			return newResult(tc, 0, "Tool is not interactive: "+tc.Name, true)
		}

		start := time.Now()
		result := it.ExecuteWithResponse(ctx, prepared.Params, response, cwd)
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newResultFromOutput(tc, 0, result)
	}
}

// RequiresUserInteraction checks if a tool call needs user approval.
// hookAllowed contains tool call IDs that were pre-approved by hooks (may be nil).
// hookForceAsk contains tool call IDs forced to prompt by PreToolUse "ask" decision (may be nil).
func RequiresUserInteraction(tc core.ToolCall, settings *config.Settings, sessionPerms *config.SessionPermissions, planMode bool, hookAllowed map[string]bool, hookForceAsk map[string]bool) bool {
	// Hook "ask" decision forces user interaction regardless of other settings
	if hookForceAsk[tc.ID] {
		return true
	}

	if planMode && coretool.IsAgentToolName(tc.Name) {
		return false
	}

	params, err := core.ParseToolInput(tc.Input)
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
		// For unknown tools (includes MCP tools), still check settings/session
		// permissions before requiring user interaction.
		if settings != nil {
			switch settings.CheckPermission(tc.Name, params, sessionPerms) {
			case config.Allow:
				return false
			case config.Deny:
				return false // denied — ProcessNext will handle denial
			}
		}
		return true
	}

	if it, ok := t.(coretool.InteractiveTool); ok && it.RequiresInteraction() {
		return true
	}

	// Check settings + session permissions first.
	// If explicitly allowed (e.g., "allow all agents this session"),
	// skip the tool's built-in permission requirement.
	if settings != nil {
		perm := settings.CheckPermission(tc.Name, params, sessionPerms)
		switch perm {
		case config.Allow:
			return false // session/settings explicitly allow — no interaction needed
		case config.Ask:
			return true
		}
	}

	if pat, ok := t.(coretool.PermissionAwareTool); ok && pat.RequiresPermission() {
		return true
	}

	return false
}


func formatPrepareError(err error) string {
	if err == nil {
		return ""
	}
	if strings.HasPrefix(err.Error(), "unknown tool: ") {
		return "Unknown tool: " + strings.TrimPrefix(err.Error(), "unknown tool: ")
	}
	return "Error parsing tool input: " + err.Error()
}

func executionContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func sendAgentProgress(hub *progress.Hub, index int, msg string) {
	if hub == nil {
		return
	}
	hub.SendForAgent(index, msg)
}


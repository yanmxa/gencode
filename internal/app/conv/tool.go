package conv

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// --- Tool state ---

// ToolExecState holds tool execution state for the TUI model.
type ToolExecState struct {
	PendingCalls []core.ToolCall
	CurrentIdx   int
	Ctx          context.Context
	Cancel       context.CancelFunc
}

// Begin initializes a fresh execution context for a tool run and returns it.
func (t *ToolExecState) Begin() context.Context {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Ctx, t.Cancel = context.WithCancel(context.Background())
	return t.Ctx
}

// Context returns the active execution context, or Background when idle.
func (t *ToolExecState) Context() context.Context {
	if t.Ctx != nil {
		return t.Ctx
	}
	return context.Background()
}

// Reset clears all tool execution state.
func (t *ToolExecState) Reset() {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.PendingCalls = nil
	t.CurrentIdx = 0
	t.Ctx = nil
	t.Cancel = nil
}

// DrainPendingCalls cancels the current context and returns any remaining
// pending tool calls (from CurrentIdx onward), then resets state.
func (t *ToolExecState) DrainPendingCalls() []core.ToolCall {
	if t.Cancel != nil {
		t.Cancel()
	}
	if t.PendingCalls == nil || t.CurrentIdx >= len(t.PendingCalls) {
		return nil
	}
	calls := t.PendingCalls[t.CurrentIdx:]
	t.Reset()
	return calls
}

// RemainingCalls returns pending tool calls from startIdx onward without modifying state.
func (t *ToolExecState) RemainingCalls(startIdx int) []core.ToolCall {
	if t.PendingCalls == nil || startIdx >= len(t.PendingCalls) {
		return nil
	}
	return t.PendingCalls[startIdx:]
}

// --- Tool execution dispatching ---

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
		Metadata: toolresult.ResultMetadata{Title: name, Icon: "plugin"},
	}, nil
}

type ExecResultMsg struct {
	Index    int
	Result   core.ToolResult
	ToolName string
}

func newExecResult(tc core.ToolCall, index int, content string, isError bool) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   core.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isError},
		ToolName: tc.Name,
	}
}

func newExecResultFromOutput(tc core.ToolCall, index int, output toolresult.ToolResult) ExecResultMsg {
	return ExecResultMsg{
		Index:    index,
		Result:   core.ToolResult{ToolCallID: tc.ID, Content: output.FormatForLLM(), IsError: !output.Success, HookResponse: output.HookResponse},
		ToolName: tc.Name,
	}
}

func ExecuteApproved(ctx context.Context, hub *ProgressHub, toolCalls []core.ToolCall, idx int, cwd string, mcpExec ...coretool.MCPExecutor) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]
	executor := coretool.MCPExecutor(defaultMCPExecutor{})
	if len(mcpExec) > 0 && mcpExec[0] != nil {
		executor = mcpExec[0]
	}

	return func() tea.Msg {
		ctx = execContext(ctx)

		prepared, err := coretool.PrepareToolCall(tc, executor)
		if err != nil {
			return newExecResult(tc, idx, formatExecPrepareError(err), true)
		}

		attachExecAgentCallbacks(ctx, hub, idx, prepared)

		start := time.Now()
		result, err := prepared.Execute(ctx, cwd, true, executor)
		if err != nil {
			if executor != nil && executor.IsMCPTool(tc.Name) {
				return newExecResult(tc, idx, "Internal error: "+err.Error(), true)
			}
			return newExecResult(tc, idx, "Internal error: unknown tool: "+tc.Name, true)
		}
		log.LogTool(tc.Name, tc.ID, time.Since(start).Milliseconds(), result.Success)
		return newExecResultFromOutput(tc, idx, result)
	}
}

func attachExecAgentCallbacks(ctx context.Context, hub *ProgressHub, idx int, prepared *coretool.PreparedToolCall) {
	if !coretool.IsAgentToolName(prepared.Call.Name) {
		return
	}

	prepared.Params["_onProgress"] = coretool.ProgressFunc(func(msg string) {
		sendExecAgentProgress(hub, idx, msg)
	})
	prepared.Params["_onQuestion"] = coretool.AskQuestionFunc(func(qctx context.Context, req *coretool.QuestionRequest) (*coretool.QuestionResponse, error) {
		if qctx == nil {
			qctx = ctx
		}
		return askExecAgentQuestion(qctx, hub, idx, req)
	})

	if getter := coretool.GetMessagesGetter(ctx); getter != nil {
		prepared.Params["_messagesGetter"] = getter
	}
}

func askExecAgentQuestion(ctx context.Context, hub *ProgressHub, idx int, req *coretool.QuestionRequest) (*coretool.QuestionResponse, error) {
	if hub == nil {
		return nil, context.Canceled
	}
	return hub.Ask(ctx, idx, req)
}

func formatExecPrepareError(err error) string {
	if err == nil {
		return ""
	}
	if strings.HasPrefix(err.Error(), "unknown tool: ") {
		return "Unknown tool: " + strings.TrimPrefix(err.Error(), "unknown tool: ")
	}
	return "Error parsing tool input: " + err.Error()
}

func execContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func sendExecAgentProgress(hub *ProgressHub, index int, msg string) {
	if hub == nil {
		return
	}
	hub.SendForAgent(index, msg)
}

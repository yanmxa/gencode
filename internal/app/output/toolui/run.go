// Tool execution: permission checks, MCP tool support.
package toolui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/core"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
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

// ExecuteApproved executes a tool that has been approved by the user.
func ExecuteApproved(ctx context.Context, hub *appoutput.ProgressHub, toolCalls []core.ToolCall, idx int, cwd string) tea.Cmd {
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

func attachAgentCallbacks(ctx context.Context, hub *appoutput.ProgressHub, idx int, prepared *coretool.PreparedToolCall) {
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

func askAgentQuestion(ctx context.Context, hub *appoutput.ProgressHub, idx int, req *coretool.QuestionRequest) (*coretool.QuestionResponse, error) {
	if hub == nil {
		return nil, context.Canceled
	}
	return hub.Ask(ctx, idx, req)
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

func sendAgentProgress(hub *appoutput.ProgressHub, index int, msg string) {
	if hub == nil {
		return
	}
	hub.SendForAgent(index, msg)
}

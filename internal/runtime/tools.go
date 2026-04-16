package runtime

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// FilterToolCallsResult is an alias for hooks.FilterToolCallsResult.
type FilterToolCallsResult = hooks.FilterToolCallsResult

// FilterToolCalls runs PreToolUse hooks. Convenience wrapper for backward compat.
func (l *Loop) FilterToolCalls(ctx context.Context, calls []core.ToolCall) (
	allowed []core.ToolCall, blocked []core.ToolResult, hookAllowed map[string]bool, additionalContext string,
) {
	r := l.FilterToolCallsEx(ctx, calls)
	return r.Allowed, r.Blocked, r.HookAllowed, r.AdditionalContext
}

// FilterToolCallsEx runs PreToolUse hooks, returning full results including ForceAsk.
func (l *Loop) FilterToolCallsEx(ctx context.Context, calls []core.ToolCall) FilterToolCallsResult {
	if l.Hooks == nil {
		return FilterToolCallsResult{Allowed: calls}
	}
	return l.Hooks.FilterToolCalls(ctx, calls, l.agentID, l.agentType)
}

// firePostToolHook fires PostToolUse or PostToolUseFailure hooks after tool execution.
func (l *Loop) firePostToolHook(ctx context.Context, tc core.ToolCall, result *core.ToolResult) {
	if l.Hooks == nil {
		return
	}

	params, _ := core.ParseToolInput(tc.Input)
	event := hooks.PostToolUse
	if result.IsError {
		event = hooks.PostToolUseFailure
	}

	toolResponse := any(result.Content)
	if result.HookResponse != nil {
		toolResponse = result.HookResponse
	}
	input := hooks.HookInput{
		ToolName:     tc.Name,
		ToolInput:    params,
		ToolUseID:    tc.ID,
		ToolResponse: toolResponse,
	}
	if l.agentID != "" {
		input.AgentID = l.agentID
		input.AgentType = l.agentType
	}
	if result.IsError {
		input.Error = result.Content
	}

	l.Hooks.ExecuteAsync(event, input)
}

// ExecTool executes a single tool call, consulting the Permission checker.
// Rejected tools return an error result; Prompt decisions are auto-approved.
func (l *Loop) ExecTool(ctx context.Context, tc core.ToolCall) *core.ToolResult {
	prepared, err := tool.PrepareToolCall(tc, mcpAdapter{caller: l.MCP})
	if err != nil {
		return core.ErrorResult(tc, fmt.Sprintf("Error parsing tool input: %v", err))
	}

	if tool.IsAgentToolName(prepared.Call.Name) {
		snapshot := make([]core.Message, len(l.messages))
		copy(snapshot, l.messages)
		prepared.Params["_messagesGetter"] = tool.MessagesGetter(func() []core.Message {
			return snapshot
		})
	}

	decision := permission.Permit
	if l.Permission != nil {
		decision = l.Permission.Check(prepared.Call.Name, prepared.Params)
	}

	if decision == permission.Reject {
		return core.ErrorResult(tc, fmt.Sprintf("Tool %s is not permitted in this mode", tc.Name))
	}

	return l.runTool(ctx, prepared)
}

func (l *Loop) runTool(ctx context.Context, prepared *tool.PreparedToolCall) *core.ToolResult {
	cwd := l.Cwd

	if it, ok := prepared.Tool.(tool.InteractiveTool); ok && it.RequiresInteraction() {
		req, err := it.PrepareInteraction(ctx, prepared.Params, cwd)
		if err != nil {
			return core.ErrorResult(prepared.Call, fmt.Sprintf("Error preparing interactive tool: %v", err))
		}

		questionReq, ok := req.(*tool.QuestionRequest)
		if !ok {
			return core.ErrorResult(prepared.Call, fmt.Sprintf("interactive tool %s is not supported in this runtime", prepared.Call.Name))
		}
		if l.questionHandler == nil {
			return core.ErrorResult(prepared.Call, fmt.Sprintf("interactive tool %s requires a question handler in this runtime", prepared.Call.Name))
		}

		resp, err := l.questionHandler(ctx, questionReq)
		if err != nil {
			return core.ErrorResult(prepared.Call, fmt.Sprintf("Question prompt failed: %v", err))
		}
		if resp == nil {
			return core.ErrorResult(prepared.Call, "Question prompt failed: no response received")
		}

		toolResult := it.ExecuteWithResponse(ctx, prepared.Params, resp, cwd)
		return &core.ToolResult{
			ToolCallID:   prepared.Call.ID,
			ToolName:     prepared.Call.Name,
			Content:      toolResult.FormatForLLM(),
			IsError:      !toolResult.Success,
			HookResponse: toolResult.HookResponse,
		}
	}

	toolResult, err := prepared.Execute(ctx, cwd, true, mcpAdapter{caller: l.MCP})
	if err != nil {
		if prepared.IsMCP {
			return core.ErrorResult(prepared.Call, fmt.Sprintf("MCP tool error: %v", err))
		}
		return core.ErrorResult(prepared.Call, fmt.Sprintf("Unknown tool: %s", prepared.Call.Name))
	}

	log.Logger().Debug("Tool executed",
		zap.String("tool", prepared.Call.Name),
		zap.Bool("success", toolResult.Success),
	)

	return &core.ToolResult{
		ToolCallID:   prepared.Call.ID,
		ToolName:     prepared.Call.Name,
		Content:      toolResult.FormatForLLM(),
		IsError:      !toolResult.Success,
		HookResponse: toolResult.HookResponse,
	}
}

type mcpAdapter struct {
	caller MCPCaller
}

func (a mcpAdapter) IsMCPTool(name string) bool {
	return a.caller != nil && a.caller.IsMCPTool(name)
}

func (a mcpAdapter) ExecuteMCP(ctx context.Context, name string, params map[string]any) (toolresult.ToolResult, error) {
	if a.caller == nil {
		return toolresult.ToolResult{}, fmt.Errorf("MCP caller not configured")
	}

	content, isError, err := a.caller.CallTool(ctx, name, params)
	if err != nil {
		return toolresult.ToolResult{}, err
	}

	return toolresult.ToolResult{
		Success: !isError,
		Output:  content,
		Metadata: toolresult.ResultMetadata{
			Title: name,
		},
	}, nil
}

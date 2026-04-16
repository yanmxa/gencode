package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/tool"
)

// Note: updateTool has been removed — tool execution results (ExecResultMsg,
// ExecDoneMsg) and progress (UpdateMsg, CheckTickMsg) are now routed through
// updateAgent in agent_events.go. QuestionMsg is handled by updateMode.

// Overflow constants and utility functions (persistToolResultOverflow,
// getHookResponseString, isTaskTool) moved to agent_events.go.


func (m *model) applyToolResultSideEffects(msg toolui.ExecResultMsg) {
	if tool.IsAgentToolName(msg.ToolName) {
		delete(m.output.TaskProgress, msg.Index)
	}
	m.syncBackgroundTaskTracker(tooluiExecResultLike{ToolName: msg.ToolName, Result: msg.Result})
	if isTaskTool(msg.ToolName) {
		m.conv.TurnsSinceLastTaskTool = 0
	}
	m.applyEnvironmentSideEffects(msg)
	m.firePostToolUseHook(msg)
}

func (m *model) applyEnvironmentSideEffects(msg toolui.ExecResultMsg) {
	if msg.Result.IsError {
		return
	}

	resp, ok := msg.Result.HookResponse.(map[string]any)
	if !ok {
		return
	}

	switch msg.ToolName {
	case "Bash":
		if newCwd := getHookResponseString(resp, "cwd"); newCwd != "" {
			m.changeCwd(newCwd)
		}
	case tool.ToolEnterWorktree:
		if worktreePath := getHookResponseString(resp, "worktreePath"); worktreePath != "" {
			m.changeCwd(worktreePath)
		}
	case tool.ToolExitWorktree:
		if restoredPath := getHookResponseString(resp, "restoredPath"); restoredPath != "" {
			m.changeCwd(restoredPath)
		}
	case "Write", "Edit":
		if filePath := getHookResponseString(resp, "filePath"); filePath != "" {
			m.fireFileChanged(filePath, msg.ToolName)
			if m.fileCache != nil {
				m.fileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := getHookResponseString(fileData, "filePath"); filePath != "" {
				if m.fileCache != nil {
					m.fileCache.Touch(filePath)
				}
			}
		}
	}
}

func (m *model) appendToolResultMessage(toolName string, result *core.ToolResult) {
	m.persistToolResultOverflow(result)
	m.conv.Append(core.ChatMessage{
		Role:       core.RoleUser,
		ToolResult: result,
		ToolName:   toolName,
	})
}

func (m *model) bufferParallelToolResult(msg toolui.ExecResultMsg) {
	if m.tool.ParallelResults == nil {
		m.tool.ParallelResults = make(map[int]core.ToolResult)
	}
	m.tool.ParallelResults[msg.Index] = msg.Result
	m.tool.ParallelCount++
}

func (m *model) handleStartToolExecution(toolCalls []core.ToolCall) tea.Cmd {
	execCtx := m.tool.Begin()
	// Inject messages getter for fork support in Agent tool
	execCtx = tool.WithMessagesGetter(execCtx, func() []core.Message {
		msgs := m.conv.ConvertToProvider()
		return msgs
	})
	m.tool.Ctx = execCtx
	m.tool.PendingCalls = m.filterToolCallsWithHooks(execCtx, toolCalls)
	m.tool.CurrentIdx = 0

	if len(m.tool.PendingCalls) == 0 {
		m.injectDeferredHookContext()
		m.tool.Reset()
		return m.continueOutbox()
	}

	if len(m.tool.PendingCalls) > 1 && m.canRunToolsInParallel(m.tool.PendingCalls) {
		m.tool.Parallel = true
		m.tool.ParallelResults = make(map[int]core.ToolResult)
		m.tool.ParallelCount = 0
	}

	return toolui.ExecuteParallel(execCtx, m.output.ProgressHub, m.tool.PendingCalls, m.cwd, m.settings, m.mode.SessionPermissions, m.mode.Enabled, m.tool.HookAllowed, m.tool.HookForceAsk)
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []core.ToolCall) bool {
	for _, tc := range toolCalls {
		if toolui.RequiresUserInteraction(tc, m.settings, m.mode.SessionPermissions, m.mode.Enabled, m.tool.HookAllowed, m.tool.HookForceAsk) {
			return false
		}
	}
	return true
}


// filterToolCallsWithHooks runs PreToolUse hooks and filters blocked tools.
func (m *model) filterToolCallsWithHooks(ctx context.Context, toolCalls []core.ToolCall) []core.ToolCall {
	result := m.hookEngine.FilterToolCalls(ctx, toolCalls, "", "")
	m.tool.HookAllowed = result.HookAllowed
	m.tool.HookForceAsk = result.HookForceAsk

	// Defer additional context injection until after all tool results
	if result.AdditionalContext != "" {
		m.tool.HookContext = result.AdditionalContext
	}

	// Add blocked results as chat messages
	for _, br := range result.Blocked {
		m.conv.Append(core.ChatMessage{
			Role:     core.RoleUser,
			ToolName: br.ToolName,
			ToolResult: &core.ToolResult{
				ToolCallID: br.ToolCallID,
				Content:    br.Content,
				IsError:    br.IsError,
			},
		})
	}

	return result.Allowed
}

// injectDeferredHookContext appends any deferred AdditionalContext from
// PreToolUse hooks as a user message, after all tool results have been added.
func (m *model) injectDeferredHookContext() {
	if m.tool.HookContext != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: m.tool.HookContext,
		})
	}
}

// firePostToolUseHook fires the PostToolUse or PostToolUseFailure hook for a tool result.
func (m *model) firePostToolUseHook(msg toolui.ExecResultMsg) {
	if m.hookEngine == nil {
		return
	}
	eventType := hooks.PostToolUse
	if msg.Result.IsError {
		eventType = hooks.PostToolUseFailure
	}
	toolResponse := any(msg.Result.Content)
	if msg.Result.HookResponse != nil {
		toolResponse = msg.Result.HookResponse
	}
	input := hooks.HookInput{
		ToolName:     msg.ToolName,
		ToolUseID:    msg.Result.ToolCallID,
		ToolResponse: toolResponse,
	}
	if msg.Index >= 0 && msg.Index < len(m.tool.PendingCalls) {
		if params, err := core.ParseToolInput(m.tool.PendingCalls[msg.Index].Input); err == nil {
			input.ToolInput = params
		}
	}
	if msg.Result.IsError {
		input.Error = msg.Result.Content
	}
	m.hookEngine.ExecuteAsync(eventType, input)
}

// isExpectedToolResult checks whether an incoming tool result belongs to the
// current set of pending tool calls. Returns false for stale results from
// cancelled executions that arrive after new tool calls have started.
func (m *model) isExpectedToolResult(msg toolui.ExecResultMsg) bool {
	if m.tool.PendingCalls == nil {
		return false
	}
	if msg.Index < 0 || msg.Index >= len(m.tool.PendingCalls) {
		return false
	}
	return m.tool.PendingCalls[msg.Index].ID == msg.Result.ToolCallID
}

// isTaskTool, persistToolResultOverflow moved to agent_events.go

package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/messageconv"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

// updateTool routes tool execution and progress messages.
func (m *model) updateTool(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case toolui.ExecResultMsg:
		c := m.handleToolResult(msg)
		return c, true
	case toolui.ExecDoneMsg:
		c := m.handleAllToolsCompleted()
		return c, true
	case progress.UpdateMsg:
		c := m.handleTaskProgress(msg)
		return c, true
	case progress.QuestionMsg:
		c := m.handleQuestionRequest(appmode.QuestionRequestMsg{
			Request: msg.Request,
			Reply:   msg.Reply,
		})
		return c, true
	case progress.CheckTickMsg:
		c := m.handleTaskProgressTick()
		return c, true
	}
	return nil, false
}

const (
	// toolResultOverflowThreshold is the size in bytes above which tool results
	// are persisted to disk and replaced with a truncated preview.
	toolResultOverflowThreshold = 100_000 // 100KB

	// toolResultPreviewSize is the number of bytes to keep as an inline preview
	// when a tool result exceeds the overflow threshold.
	toolResultPreviewSize = 10_000 // 10KB
)

func (m *model) handleToolResult(msg toolui.ExecResultMsg) tea.Cmd {
	// Discard stale results from cancelled/completed tool executions.
	// This happens when the user cancels (Esc) during tool execution;
	// the background goroutine may still deliver results after the
	// conversation has moved on to new tool calls.
	if !m.isExpectedToolResult(msg) {
		return nil
	}

	// Check if we're in parallel mode
	if m.tool.Parallel {
		return m.handleParallelToolResult(msg)
	}

	prevCwd := m.cwd
	m.applyToolResultSideEffects(msg)

	r := msg.Result
	m.appendToolResultMessage(msg.ToolName, &r)
	if m.shouldReplanAfterCwdChange(prevCwd) {
		m.cancelRemainingToolCalls(m.tool.CurrentIdx + 1)
		m.tool.Reset()
		commitCmds := m.commitMessages()
		commitCmds = append(commitCmds, m.startContinueStream())
		return tea.Batch(commitCmds...)
	}
	m.tool.CurrentIdx++
	commitCmds := m.commitMessages()
	nextTool := toolui.ProcessNext(m.tool.Context(), m.output.ProgressHub, m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd, m.settings, m.mode.SessionPermissions, m.tool.HookAllowed, m.tool.HookForceAsk)
	return tea.Batch(append(commitCmds, nextTool)...)
}

func (m *model) shouldReplanAfterCwdChange(prevCwd string) bool {
	if prevCwd == "" || m.cwd == prevCwd {
		return false
	}
	return m.tool.CurrentIdx+1 < len(m.tool.PendingCalls)
}

func (m *model) handleParallelToolResult(msg toolui.ExecResultMsg) tea.Cmd {
	m.applyToolResultSideEffects(msg)
	m.bufferParallelToolResult(msg)

	// Check if all results are in
	if m.tool.ParallelCount >= len(m.tool.PendingCalls) {
		return m.completeParallelExecution()
	}

	// More results pending
	return nil
}

func (m *model) completeParallelExecution() tea.Cmd {
	for i := 0; i < len(m.tool.PendingCalls); i++ {
		tc := m.tool.PendingCalls[i]
		if result, ok := m.tool.ParallelResults[i]; ok {
			m.appendToolResultMessage(tc.Name, &result)
		}
	}

	m.injectDeferredHookContext()
	m.output.TaskProgress = nil // clear all agent progress
	m.tool.Reset()
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.startContinueStream())
	return tea.Batch(commitCmds...)
}

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

func getHookResponseString(resp map[string]any, key string) string {
	if value, ok := resp[key].(string); ok {
		return value
	}
	return ""
}

func (m *model) appendToolResultMessage(toolName string, result *message.ToolResult) {
	m.persistToolResultOverflow(result)
	m.conv.Append(message.ChatMessage{
		Role:       message.RoleUser,
		ToolResult: result,
		ToolName:   toolName,
	})
}

func (m *model) bufferParallelToolResult(msg toolui.ExecResultMsg) {
	if m.tool.ParallelResults == nil {
		m.tool.ParallelResults = make(map[int]message.ToolResult)
	}
	m.tool.ParallelResults[msg.Index] = msg.Result
	m.tool.ParallelCount++
}

func (m *model) handleStartToolExecution(toolCalls []message.ToolCall) tea.Cmd {
	execCtx := m.tool.Begin()
	// Inject messages getter for fork support in Agent tool
	execCtx = tool.WithMessagesGetter(execCtx, func() []core.Message {
		msgs := m.conv.ConvertToProvider()
		return messageconv.ToCoreSlice(msgs)
	})
	m.tool.Ctx = execCtx
	m.tool.PendingCalls = m.filterToolCallsWithHooks(execCtx, toolCalls)
	m.tool.CurrentIdx = 0

	if len(m.tool.PendingCalls) == 0 {
		m.injectDeferredHookContext()
		m.tool.Reset()
		return m.startContinueStream()
	}

	if len(m.tool.PendingCalls) > 1 && m.canRunToolsInParallel(m.tool.PendingCalls) {
		m.tool.Parallel = true
		m.tool.ParallelResults = make(map[int]message.ToolResult)
		m.tool.ParallelCount = 0
	}

	return toolui.ExecuteParallel(execCtx, m.output.ProgressHub, m.tool.PendingCalls, m.cwd, m.settings, m.mode.SessionPermissions, m.mode.Enabled, m.tool.HookAllowed, m.tool.HookForceAsk)
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []message.ToolCall) bool {
	for _, tc := range toolCalls {
		if toolui.RequiresUserInteraction(tc, m.settings, m.mode.SessionPermissions, m.mode.Enabled, m.tool.HookAllowed, m.tool.HookForceAsk) {
			return false
		}
	}
	return true
}

func (m *model) handleAllToolsCompleted() tea.Cmd {
	m.injectDeferredHookContext()
	m.tool.Reset()
	return m.startContinueStream()
}

// filterToolCallsWithHooks runs PreToolUse hooks and filters blocked tools.
func (m *model) filterToolCallsWithHooks(ctx context.Context, toolCalls []message.ToolCall) []message.ToolCall {
	result := m.hookEngine.FilterToolCalls(ctx, toolCalls, "", "")
	m.tool.HookAllowed = result.HookAllowed
	m.tool.HookForceAsk = result.HookForceAsk

	// Defer additional context injection until after all tool results
	if result.AdditionalContext != "" {
		m.tool.HookContext = result.AdditionalContext
	}

	// Add blocked results as chat messages
	for _, br := range result.Blocked {
		m.conv.Append(message.ChatMessage{
			Role:     message.RoleUser,
			ToolName: br.ToolName,
			ToolResult: &message.ToolResult{
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
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleUser,
			Content: m.tool.HookContext,
		})
	}
}

// firePostToolUseHook fires the PostToolUse or PostToolUseFailure hook for a tool result.
func (m *model) firePostToolUseHook(msg toolui.ExecResultMsg) {
	if m.hookEngine == nil {
		return
	}
	eventType := core.PostToolUse
	if msg.Result.IsError {
		eventType = core.PostToolUseFailure
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
		if params, err := message.ParseToolInput(m.tool.PendingCalls[msg.Index].Input); err == nil {
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

// isTaskTool returns true if the tool name is a task management tool.
func isTaskTool(name string) bool {
	switch name {
	case tool.ToolTaskCreate, tool.ToolTaskGet, tool.ToolTaskUpdate, tool.ToolTaskList:
		return true
	}
	return false
}

// persistToolResultOverflow checks if a tool result exceeds the overflow threshold
// and, if so, persists the full content to disk and replaces it with a truncated preview.
// If persistence fails, the result is still truncated to prevent bloating the context.
func (m *model) persistToolResultOverflow(result *message.ToolResult) {
	if len(result.Content) <= toolResultOverflowThreshold {
		return
	}

	// Truncate at a valid UTF-8 boundary to avoid producing invalid output.
	cutoff := toolResultPreviewSize
	if cutoff > len(result.Content) {
		cutoff = len(result.Content)
	}
	for cutoff > 0 && !utf8.RuneStart(result.Content[cutoff]) {
		cutoff--
	}
	preview := result.Content[:cutoff]

	// Try to persist the full content to disk. If this fails, we still truncate
	// the result to prevent 100KB+ content from bloating the conversation context.
	persisted := false
	if err := m.ensureSessionStore(); err == nil && m.session.CurrentID != "" {
		if err := m.session.Store.PersistToolResult(m.session.CurrentID, result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}

	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.session.CurrentID, result.ToolCallID)
	} else {
		result.Content = fmt.Sprintf("%s\n\n[Output truncated from %d bytes — full content not persisted]", preview, len(result.Content))
	}
}

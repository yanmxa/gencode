// Stream processing: stream chunks, completion, errors, continuation,
// tool results (sequential and parallel), tool filtering, spinner ticks.
package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
)

func (m *model) handleToolResult(msg ResultMsg) (tea.Model, tea.Cmd) {
	// Check if we're in parallel mode
	if m.toolExec.parallel {
		return m.handleParallelToolResult(msg)
	}

	// Clear task progress for this agent when Task tool completes
	if msg.ToolName == "Task" {
		delete(m.taskProgress, msg.Index)
	}

	// Execute PostToolUse or PostToolUseFailure hook asynchronously
	if m.hookEngine != nil {
		eventType := hooks.PostToolUse
		if msg.Result.IsError {
			eventType = hooks.PostToolUseFailure
		}
		input := hooks.HookInput{
			ToolName:     msg.ToolName,
			ToolResponse: msg.Result.Content,
		}
		if msg.Result.IsError {
			input.Error = msg.Result.Content
		}
		m.hookEngine.ExecuteAsync(eventType, input)
	}

	// Sequential mode - original behavior
	r := msg.Result
	m.messages = append(m.messages, chatMessage{
		role:       roleUser,
		toolResult: &r,
		toolName:   msg.ToolName,
	})
	m.toolExec.currentIdx++
	commitCmds := m.commitMessages()
	nextTool := ProcessNext(m.toolExec.pendingCalls, m.toolExec.currentIdx, m.cwd, m.settings, m.sessionPermissions)
	return m, tea.Batch(append(commitCmds, nextTool)...)
}

func (m *model) handleParallelToolResult(msg ResultMsg) (tea.Model, tea.Cmd) {
	// Store result in the parallel results map
	if m.toolExec.parallelResults == nil {
		m.toolExec.parallelResults = make(map[int]message.ToolResult)
	}
	m.toolExec.parallelResults[msg.Index] = msg.Result
	m.toolExec.parallelCount++

	// Check if all results are in
	if m.toolExec.parallelCount >= len(m.toolExec.pendingCalls) {
		return m.completeParallelExecution()
	}

	// More results pending
	return m, nil
}

func (m *model) completeParallelExecution() (tea.Model, tea.Cmd) {
	for i := 0; i < len(m.toolExec.pendingCalls); i++ {
		tc := m.toolExec.pendingCalls[i]
		if result, ok := m.toolExec.parallelResults[i]; ok {
			m.messages = append(m.messages, chatMessage{
				role:       roleUser,
				toolResult: &result,
				toolName:   tc.Name,
			})
		}
	}

	m.taskProgress = nil // clear all agent progress
	m.toolExec.Reset()
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.continueWithToolResults())
	return m, tea.Batch(commitCmds...)
}

func (m *model) handleStartToolExecution(msg StartMsg) (tea.Model, tea.Cmd) {
	m.toolExec.pendingCalls = m.filterToolCallsWithHooks(msg.ToolCalls)
	m.toolExec.currentIdx = 0

	if len(m.toolExec.pendingCalls) == 0 {
		return m, m.continueWithToolResults()
	}

	cmd := ExecuteParallel(m.toolExec.pendingCalls, m.cwd, m.settings, m.sessionPermissions, m.planMode)

	if len(m.toolExec.pendingCalls) > 1 && m.canRunToolsInParallel(m.toolExec.pendingCalls) {
		m.toolExec.parallel = true
		m.toolExec.parallelResults = make(map[int]message.ToolResult)
		m.toolExec.parallelCount = 0
	}

	return m, cmd
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []message.ToolCall) bool {
	for _, tc := range toolCalls {
		if RequiresUserInteraction(tc, m.settings, m.sessionPermissions, m.planMode) {
			return false
		}
	}
	return true
}

func (m *model) handleAllToolsCompleted() (tea.Model, tea.Cmd) {
	m.toolExec.Reset()
	return m, m.continueWithToolResults()
}

// filterToolCallsWithHooks runs PreToolUse hooks and filters blocked tools.
func (m *model) filterToolCallsWithHooks(toolCalls []message.ToolCall) []message.ToolCall {
	allowed, blocked := m.loop.FilterToolCalls(context.Background(), toolCalls)

	// Add blocked results as chat messages
	for _, br := range blocked {
		m.messages = append(m.messages, chatMessage{
			role:     roleUser,
			toolName: br.ToolName,
			toolResult: &message.ToolResult{
				ToolCallID: br.ToolCallID,
				Content:    br.Content,
				IsError:    br.IsError,
			},
		})
	}

	return allowed
}

func (m *model) handleStreamChunk(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	if msg.buildingToolName != "" {
		m.stream.buildingTool = msg.buildingToolName
	}

	if msg.err != nil {
		return m.handleStreamError(msg.err)
	}

	if msg.done {
		return m.handleStreamDone(msg)
	}

	// Streaming text/thinking chunks: update the message content
	m.appendToLastMessage(msg.text, msg.thinking)
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
}

// handleStreamDone processes a completed stream.
func (m *model) handleStreamDone(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	m.stream.buildingTool = ""

	if msg.usage != nil {
		m.lastInputTokens = msg.usage.InputTokens
		m.lastOutputTokens = msg.usage.OutputTokens
	}

	if len(msg.toolCalls) > 0 {
		m.setLastMessageToolCalls(msg.toolCalls)
		commitCmds := m.commitMessages()

		if m.shouldAutoCompact() {
			m.compact.autoContinue = true // Auto-continue after compaction
			commitCmds = append(commitCmds, m.triggerAutoCompact())
			return m, tea.Batch(commitCmds...)
		}

		commitCmds = append(commitCmds, m.executeTools(msg.toolCalls))
		return m, tea.Batch(commitCmds...)
	}

	m.stream.Stop()

	commitCmds := m.commitMessages()

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{})
	}

	_ = m.saveSession()

	if m.shouldAutoCompact() {
		commitCmds = append(commitCmds, m.triggerAutoCompact())
	}

	return m, tea.Batch(commitCmds...)
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) (tea.Model, tea.Cmd) {
	// If "prompt too long", trigger auto-compact and retry
	if strings.Contains(err.Error(), "prompt is too long") && len(m.messages) >= 3 {
		m.removeEmptyLastAssistantMessage()
		m.stream.Stop()
		m.compact.autoContinue = true // Auto-continue after compaction
		return m, m.triggerAutoCompact()
	}

	m.appendErrorToLastMessage(err)
	m.stream.Stop()
	return m, tea.Batch(m.commitMessages()...)
}

// appendToLastMessage appends text and thinking content to the last message.
func (m *model) appendToLastMessage(text, thinking string) {
	if len(m.messages) == 0 {
		return
	}
	idx := len(m.messages) - 1
	if thinking != "" {
		m.messages[idx].thinking += thinking
	}
	if text != "" {
		m.messages[idx].content += text
	}
}

// setLastMessageToolCalls sets tool calls on the last message.
func (m *model) setLastMessageToolCalls(calls []message.ToolCall) {
	if len(m.messages) > 0 {
		m.messages[len(m.messages)-1].toolCalls = calls
	}
}

// appendErrorToLastMessage appends an error to the last message content.
func (m *model) appendErrorToLastMessage(err error) {
	if len(m.messages) > 0 {
		idx := len(m.messages) - 1
		m.messages[idx].content += "\n[Error: " + err.Error() + "]"
	}
}

// removeEmptyLastAssistantMessage removes the last message if it's an empty assistant message.
func (m *model) removeEmptyLastAssistantMessage() {
	if len(m.messages) > 0 {
		last := m.messages[len(m.messages)-1]
		if last.role == roleAssistant && last.content == "" {
			m.messages = m.messages[:len(m.messages)-1]
		}
	}
}

func (m *model) handleStreamContinue(msg streamContinueMsg) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.stream.cancel = cancel

	// Commit any pending messages before starting new stream
	commitCmds := m.commitMessages()

	m.messages = append(m.messages, chatMessage{role: roleAssistant, content: ""})

	// Configure loop with current state and set messages
	m.configureLoop(m.buildExtraContext())
	m.loop.SetMessages(msg.messages)

	m.stream.ch = m.loop.Stream(ctx)
	allCmds := append(commitCmds, m.waitForChunk(), m.spinner.Tick)
	return m, tea.Batch(allCmds...)
}

func (m *model) handleSpinnerTick(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle token limit fetching spinner (no additional processing needed)
	if m.fetchingTokenLimits {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if !m.stream.active {
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)

	interactiveActive := m.questionPrompt.IsActive() || (m.planPrompt != nil && m.planPrompt.IsActive())
	if interactiveActive {
		return m, cmd
	}

	// Check for Task progress updates (drains all pending messages)
	if m.hasRunningTaskTools() {
		m.drainTaskProgress()
	}

	// View() renders active content live, no viewport update needed
	return m, cmd
}

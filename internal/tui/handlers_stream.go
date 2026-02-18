package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
)

func (m *model) handleToolResult(msg toolResultMsg) (tea.Model, tea.Cmd) {
	// Check if we're in parallel mode
	if m.parallelMode {
		return m.handleParallelToolResult(msg)
	}

	// Clear task progress when Task tool completes
	if msg.toolName == "Task" {
		m.taskProgress = nil
	}

	// Execute PostToolUse or PostToolUseFailure hook asynchronously
	if m.hookEngine != nil {
		eventType := hooks.PostToolUse
		if msg.result.IsError {
			eventType = hooks.PostToolUseFailure
		}
		input := hooks.HookInput{
			ToolName:     msg.toolName,
			ToolResponse: msg.result.Content,
		}
		if msg.result.IsError {
			input.Error = msg.result.Content
		}
		m.hookEngine.ExecuteAsync(eventType, input)
	}

	// Sequential mode - original behavior
	r := msg.result
	m.messages = append(m.messages, chatMessage{
		role:       roleUser,
		toolResult: &r,
		toolName:   msg.toolName,
	})
	m.pendingToolIdx++
	commitCmds := m.commitMessages()
	nextTool := processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)
	return m, tea.Batch(append(commitCmds, nextTool)...)
}

func (m *model) handleParallelToolResult(msg toolResultMsg) (tea.Model, tea.Cmd) {
	// Store result in the parallel results map
	if m.parallelResults == nil {
		m.parallelResults = make(map[int]message.ToolResult)
	}
	m.parallelResults[msg.index] = msg.result
	m.parallelResultCount++

	// Check if all results are in
	if m.parallelResultCount >= len(m.pendingToolCalls) {
		return m.completeParallelExecution()
	}

	// More results pending
	return m, nil
}

func (m *model) completeParallelExecution() (tea.Model, tea.Cmd) {
	for i := 0; i < len(m.pendingToolCalls); i++ {
		tc := m.pendingToolCalls[i]
		if result, ok := m.parallelResults[i]; ok {
			m.messages = append(m.messages, chatMessage{
				role:       roleUser,
				toolResult: &result,
				toolName:   tc.Name,
			})
		}
	}

	m.resetToolState()
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.continueWithToolResults())
	return m, tea.Batch(commitCmds...)
}

func (m *model) handleStartToolExecution(msg startToolExecutionMsg) (tea.Model, tea.Cmd) {
	m.pendingToolCalls = m.filterToolCallsWithHooks(msg.toolCalls)
	m.pendingToolIdx = 0

	if len(m.pendingToolCalls) == 0 {
		return m, m.continueWithToolResults()
	}

	cmd := executeToolsParallel(m.pendingToolCalls, m.cwd, m.settings, m.sessionPermissions)

	if len(m.pendingToolCalls) > 1 && m.canRunToolsInParallel(m.pendingToolCalls) {
		m.parallelMode = true
		m.parallelResults = make(map[int]message.ToolResult)
		m.parallelResultCount = 0
	}

	return m, cmd
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []message.ToolCall) bool {
	for _, tc := range toolCalls {
		if requiresUserInteraction(tc, m.settings, m.sessionPermissions) {
			return false
		}
	}
	return true
}

func (m *model) handleAllToolsCompleted() (tea.Model, tea.Cmd) {
	m.resetToolState()
	return m, m.continueWithToolResults()
}

// resetToolState clears all pending/parallel tool execution state.
func (m *model) resetToolState() {
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0
	m.parallelMode = false
	m.parallelResults = nil
	m.parallelResultCount = 0
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
		m.buildingToolName = msg.buildingToolName
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
	m.buildingToolName = ""

	if msg.usage != nil {
		m.lastInputTokens = msg.usage.InputTokens
		m.lastOutputTokens = msg.usage.OutputTokens
	}

	if len(msg.toolCalls) > 0 {
		m.setLastMessageToolCalls(msg.toolCalls)
		commitCmds := m.commitMessages()

		if m.shouldAutoCompact() {
			commitCmds = append(commitCmds, m.triggerAutoCompact())
			return m, tea.Batch(commitCmds...)
		}

		commitCmds = append(commitCmds, m.executeTools(msg.toolCalls))
		return m, tea.Batch(commitCmds...)
	}

	m.streaming = false
	m.streamChan = nil
	m.cancelFunc = nil

	commitCmds := m.commitMessages()

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{})
	}

	_ = m.saveSession()

	if m.shouldAutoCompact() {
		commitCmds = append(commitCmds, m.triggerAutoCompact())
	}

	if len(commitCmds) > 0 {
		return m, tea.Batch(commitCmds...)
	}
	return m, nil
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) (tea.Model, tea.Cmd) {
	// If "prompt too long", trigger auto-compact and retry
	if strings.Contains(err.Error(), "prompt is too long") && len(m.messages) >= 3 {
		m.removeEmptyLastAssistantMessage()
		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		return m, m.triggerAutoCompact()
	}

	m.appendErrorToLastMessage(err)
	m.streaming = false
	m.streamChan = nil
	m.cancelFunc = nil
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
	m.cancelFunc = cancel

	// Commit any pending messages before starting new stream
	commitCmds := m.commitMessages()

	m.messages = append(m.messages, chatMessage{role: roleAssistant, content: ""})

	// Configure loop with current state and set messages
	m.configureLoop(m.buildExtraContext())
	m.loop.SetMessages(msg.messages)

	m.streamChan = m.loop.Stream(ctx)
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

	if !m.streaming {
		return m, nil
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)

	interactiveActive := m.questionPrompt.IsActive() || (m.planPrompt != nil && m.planPrompt.IsActive())
	if interactiveActive {
		return m, cmd
	}

	// Check for Task progress updates
	if m.pendingToolCalls != nil && m.pendingToolIdx < len(m.pendingToolCalls) {
		tc := m.pendingToolCalls[m.pendingToolIdx]
		if tc.Name == "Task" {
			// Check for progress messages
			ch := GetTaskProgressChan()
			select {
			case progressMsg := <-ch:
				m.taskProgress = append(m.taskProgress, progressMsg)
				if len(m.taskProgress) > 5 {
					m.taskProgress = m.taskProgress[1:]
				}
			default:
			}
		}
	}

	// View() renders active content live, no viewport update needed
	return m, cmd
}

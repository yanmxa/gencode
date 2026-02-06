package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
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
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)
}

func (m *model) handleParallelToolResult(msg toolResultMsg) (tea.Model, tea.Cmd) {
	// Store result in the parallel results map
	if m.parallelResults == nil {
		m.parallelResults = make(map[int]provider.ToolResult)
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
	// Add all results as messages in order
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

	// Reset parallel execution state
	m.parallelMode = false
	m.parallelResults = nil
	m.parallelResultCount = 0
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	return m, m.continueWithToolResults()
}

func (m *model) handleStartToolExecution(msg startToolExecutionMsg) (tea.Model, tea.Cmd) {
	m.pendingToolCalls = m.filterToolCallsWithHooks(msg.toolCalls)
	m.pendingToolIdx = 0

	if len(m.pendingToolCalls) == 0 {
		m.viewport.SetContent(m.renderMessages())
		return m, m.continueWithToolResults()
	}

	cmd := executeToolsParallel(m.pendingToolCalls, m.cwd, m.settings, m.sessionPermissions)

	if len(m.pendingToolCalls) > 1 && m.canRunToolsInParallel(m.pendingToolCalls) {
		m.parallelMode = true
		m.parallelResults = make(map[int]provider.ToolResult)
		m.parallelResultCount = 0
	}

	return m, cmd
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []provider.ToolCall) bool {
	for _, tc := range toolCalls {
		if requiresUserInteraction(tc, m.settings, m.sessionPermissions) {
			return false
		}
	}
	return true
}

func (m *model) handleAllToolsCompleted() (tea.Model, tea.Cmd) {
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0
	m.parallelMode = false
	m.parallelResults = nil
	m.parallelResultCount = 0
	return m, m.continueWithToolResults()
}

// filterToolCallsWithHooks runs PreToolUse hooks and filters blocked tools.
func (m *model) filterToolCallsWithHooks(toolCalls []provider.ToolCall) []provider.ToolCall {
	if m.hookEngine == nil {
		return toolCalls
	}

	filtered := make([]provider.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		params, _ := parseToolInput(tc.Input)
		outcome := m.hookEngine.Execute(context.Background(), hooks.PreToolUse, hooks.HookInput{
			ToolName:  tc.Name,
			ToolInput: params,
			ToolUseID: tc.ID,
		})

		if outcome.ShouldBlock {
			m.messages = append(m.messages, chatMessage{
				role:     roleUser,
				toolName: tc.Name,
				toolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Blocked by hook: " + outcome.BlockReason,
					IsError:    true,
				},
			})
			continue
		}

		if outcome.UpdatedInput != nil {
			if updated, err := encodeToolInput(outcome.UpdatedInput); err == nil {
				tc.Input = updated
			}
		}
		filtered = append(filtered, tc)
	}
	return filtered
}

func (m *model) handleStreamChunk(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	if msg.buildingToolName != "" {
		m.buildingToolName = msg.buildingToolName
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}

	if msg.done {
		m.buildingToolName = ""

		// Update token usage from the most recent API response
		if msg.usage != nil {
			m.lastInputTokens = msg.usage.InputTokens
			m.lastOutputTokens = msg.usage.OutputTokens
		}

		if len(msg.toolCalls) > 0 {
			if len(m.messages) > 0 {
				idx := len(m.messages) - 1
				m.messages[idx].toolCalls = msg.toolCalls
			}
			m.viewport.SetContent(m.renderMessages())

			return m, m.executeTools(msg.toolCalls)
		}

		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		m.viewport.SetContent(m.renderMessages())

		// Execute Stop hook asynchronously
		if m.hookEngine != nil {
			m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{})
		}

		// Auto-save session after assistant response completes
		_ = m.saveSession()

		// Check for auto-compact trigger (>= 95% context usage)
		if m.shouldAutoCompact() {
			return m, m.triggerAutoCompact()
		}
		return m, nil
	}

	if msg.err != nil {
		if len(m.messages) > 0 {
			idx := len(m.messages) - 1
			m.messages[idx].content += "\n[Error: " + msg.err.Error() + "]"
		}
		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	if len(m.messages) > 0 && msg.text != "" {
		idx := len(m.messages) - 1
		m.messages[idx].content += msg.text
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
}

func (m *model) handleStreamContinue(msg streamContinueMsg) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	m.messages = append(m.messages, chatMessage{role: roleAssistant, content: ""})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	extra := m.buildExtraContext()

	sysPrompt := system.Prompt(system.Config{
		Provider: m.llmProvider.Name(),
		Model:    msg.modelID,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Memory:   system.LoadMemory(m.cwd),
		Extra:    extra,
	})

	tools := m.getToolsForMode()

	m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
		Model:        msg.modelID,
		Messages:     msg.messages,
		MaxTokens:    m.getMaxTokens(),
		Tools:        tools,
		SystemPrompt: sysPrompt,
	})
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
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

	// Determine if viewport needs update
	needsUpdate := m.buildingToolName != "" ||
		(m.pendingToolCalls != nil && m.pendingToolIdx < len(m.pendingToolCalls))

	if !needsUpdate && len(m.messages) > 0 {
		lastMsg := m.messages[len(m.messages)-1]
		needsUpdate = lastMsg.role == roleAssistant && lastMsg.content == "" && len(lastMsg.toolCalls) == 0
	}

	if needsUpdate {
		m.viewport.SetContent(m.renderMessages())
	}
	return m, cmd
}

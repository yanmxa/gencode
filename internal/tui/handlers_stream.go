package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
)

func (m *model) handleToolResult(msg toolResultMsg) (tea.Model, tea.Cmd) {
	// Check if we're in parallel mode
	if m.parallelMode {
		return m.handleParallelToolResult(msg)
	}

	// Sequential mode - original behavior
	r := msg.result
	m.messages = append(m.messages, chatMessage{
		role:       "user",
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

func (m *model) handleTodoResult(msg todoResultMsg) (tea.Model, tea.Cmd) {
	// Check if we're in parallel mode
	if m.parallelMode {
		return m.handleParallelTodoResult(msg)
	}

	// Sequential mode - original behavior
	r := msg.result
	m.messages = append(m.messages, chatMessage{
		role:       "user",
		toolResult: &r,
		toolName:   msg.toolName,
		todos:      msg.todos,
	})
	m.todoPanel.Update(msg.todos)
	m.todoPanel.SetWidth(m.width)
	m.pendingToolIdx++
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)
}

func (m *model) handleParallelTodoResult(msg todoResultMsg) (tea.Model, tea.Cmd) {
	// Store result in the parallel results map
	if m.parallelResults == nil {
		m.parallelResults = make(map[int]provider.ToolResult)
	}
	m.parallelResults[msg.index] = msg.result
	m.parallelResultCount++

	// Collect todos
	m.parallelTodos = append(m.parallelTodos, msg.todos...)

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
				role:       "user",
				toolResult: &result,
				toolName:   tc.Name,
			})
		}
	}

	// Update todo panel if we have todos
	if len(m.parallelTodos) > 0 {
		m.todoPanel.Update(m.parallelTodos)
		m.todoPanel.SetWidth(m.width)
	}

	// Reset parallel execution state
	m.parallelMode = false
	m.parallelResults = nil
	m.parallelTodos = nil
	m.parallelResultCount = 0
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	return m, m.continueWithToolResults()
}

func (m *model) handleStartToolExecution(msg startToolExecutionMsg) (tea.Model, tea.Cmd) {
	m.pendingToolCalls = msg.toolCalls
	m.pendingToolIdx = 0

	// Try parallel execution
	cmd := executeToolsParallel(m.pendingToolCalls, m.cwd, m.settings, m.sessionPermissions)

	// Check if parallel execution was used by examining if tea.Batch was returned
	// If it's parallel, we need to set up tracking
	if len(msg.toolCalls) > 1 && m.canRunToolsInParallel(msg.toolCalls) {
		m.parallelMode = true
		m.parallelResults = make(map[int]provider.ToolResult)
		m.parallelTodos = nil
		m.parallelResultCount = 0
	}

	return m, cmd
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []provider.ToolCall) bool {
	for _, tc := range toolCalls {
		params, err := parseToolInput(tc.Input)
		if err != nil {
			return false
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return false
		}

		// Check settings
		if m.settings != nil {
			permResult := m.settings.CheckPermission(tc.Name, params, m.sessionPermissions)
			if permResult == config.PermissionAsk {
				return false
			}
		}

		// Check permission-aware tool
		if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
			return false
		}

		// Check interactive tool
		if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
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
	m.parallelTodos = nil
	m.parallelResultCount = 0
	return m, m.continueWithToolResults()
}

func (m *model) handleStreamChunk(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	if msg.buildingToolName != "" {
		m.buildingToolName = msg.buildingToolName
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}

	if msg.done {
		m.buildingToolName = ""

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

	m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	// Build extra context for system prompt
	var extra []string

	// Keep available skills metadata for active skills (don't re-inject full skill instructions)
	if skill.DefaultRegistry != nil {
		if metadata := skill.DefaultRegistry.GetAvailableSkillsPrompt(); metadata != "" {
			extra = append(extra, metadata)
		}
	}

	sysPrompt := system.Prompt(system.Config{
		Provider: m.llmProvider.Name(),
		Model:    msg.modelID,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Extra:    extra,
	})

	tools := m.getToolsForMode()

	m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
		Model:        msg.modelID,
		Messages:     msg.messages,
		MaxTokens:    defaultMaxTokens,
		Tools:        tools,
		SystemPrompt: sysPrompt,
	})
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
}

func (m *model) handleSpinnerTick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.streaming {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		interactiveActive := m.questionPrompt.IsActive() || (m.planPrompt != nil && m.planPrompt.IsActive())
		if interactiveActive {
			return m, cmd
		}

		if m.buildingToolName != "" {
			m.viewport.SetContent(m.renderMessages())
		} else if m.pendingToolCalls != nil && m.pendingToolIdx < len(m.pendingToolCalls) {
			m.viewport.SetContent(m.renderMessages())
		} else if len(m.messages) > 0 {
			lastMsg := m.messages[len(m.messages)-1]
			if lastMsg.role == "assistant" && lastMsg.content == "" && len(lastMsg.toolCalls) == 0 {
				m.viewport.SetContent(m.renderMessages())
			}
		}
		return m, cmd
	}
	return m, nil
}

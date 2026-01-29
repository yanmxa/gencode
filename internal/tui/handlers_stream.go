package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
)

func (m *model) handleToolResult(msg toolResultMsg) (tea.Model, tea.Cmd) {
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

func (m *model) handleTodoResult(msg todoResultMsg) (tea.Model, tea.Cmd) {
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

func (m *model) handleStartToolExecution(msg startToolExecutionMsg) (tea.Model, tea.Cmd) {
	m.pendingToolCalls = msg.toolCalls
	m.pendingToolIdx = 0
	return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)
}

func (m *model) handleAllToolsCompleted() (tea.Model, tea.Cmd) {
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0
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

	sysPrompt := system.Prompt(system.Config{
		Provider: m.llmProvider.Name(),
		Model:    msg.modelID,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
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

package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
)

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.planPrompt != nil && m.planPrompt.IsActive() {
		if !m.planPrompt.IsEditing() {
			switch msg.Type {
			case tea.KeyPgUp, tea.KeyCtrlU:
				m.viewport.HalfPageUp()
				return m, nil
			case tea.KeyPgDown, tea.KeyCtrlD:
				m.viewport.HalfPageDown()
				return m, nil
			}
		}
		cmd := m.planPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.questionPrompt.IsActive() {
		cmd := m.questionPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.permissionPrompt.IsActive() {
		cmd := m.permissionPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.enterPlanPrompt.IsActive() {
		cmd := m.enterPlanPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.selector.IsActive() {
		cmd := m.selector.HandleKeypress(msg)
		return m, cmd
	}

	if m.toolSelector.IsActive() {
		cmd := m.toolSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.skillSelector.IsActive() {
		cmd := m.skillSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.suggestions.IsVisible() {
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			m.suggestions.MoveUp()
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN:
			m.suggestions.MoveDown()
			return m, nil
		case tea.KeyTab, tea.KeyEnter:
			if selected := m.suggestions.GetSelected(); selected != "" {
				m.textarea.SetValue(selected + " ")
				m.textarea.CursorEnd()
				m.suggestions.Hide()
			}
			return m, nil
		case tea.KeyEsc:
			m.suggestions.Hide()
			return m, nil
		}
	}

	if msg.Type == tea.KeyShiftTab {
		if !m.streaming && !m.permissionPrompt.IsActive() &&
			!m.questionPrompt.IsActive() &&
			(m.planPrompt == nil || !m.planPrompt.IsActive()) &&
			!m.selector.IsActive() && !m.suggestions.IsVisible() {
			m.cycleOperationMode()
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
	}

	if msg.Type == tea.KeyCtrlO {
		return m.handleCtrlO()
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.textarea.Value() != "" {
			m.textarea.Reset()
			m.textarea.SetHeight(minTextareaHeight)
			m.historyIndex = -1
			return m, nil
		}
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		return m, tea.Quit

	case tea.KeyEsc:
		if m.suggestions.IsVisible() {
			m.suggestions.Hide()
			return m, nil
		}
		if m.streaming && m.cancelFunc != nil {
			return m.handleStreamCancel()
		}
		return m, nil

	case tea.KeyUp:
		if m.textarea.Line() == 0 {
			return m.handleHistoryUp()
		}

	case tea.KeyDown:
		lines := strings.Count(m.textarea.Value(), "\n")
		if m.textarea.Line() == lines {
			return m.handleHistoryDown()
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.textarea.InsertString("\n")
			m.updateTextareaHeight()
			return m, nil
		}
		return m.handleSubmit()
	}

	// Return nil, nil to let textarea handle the input
	return nil, nil
}

func (m *model) handleCtrlO() (tea.Model, tea.Cmd) {
	if m.permissionPrompt != nil && m.permissionPrompt.IsActive() {
		if m.permissionPrompt.diffPreview != nil {
			m.permissionPrompt.diffPreview.ToggleExpand()
		}
		if m.permissionPrompt.bashPreview != nil {
			m.permissionPrompt.bashPreview.ToggleExpand()
		}
		return m, nil
	}

	now := time.Now()
	if now.Sub(m.lastCtrlOTime) < doubleTapThreshold {
		anyExpanded := false
		for _, msg := range m.messages {
			if (msg.toolResult != nil && msg.expanded) || (len(msg.toolCalls) > 0 && msg.toolCallsExpanded) {
				anyExpanded = true
				break
			}
		}
		for i := range m.messages {
			if m.messages[i].toolResult != nil {
				m.messages[i].expanded = !anyExpanded
			}
			if len(m.messages[i].toolCalls) > 0 {
				m.messages[i].toolCallsExpanded = !anyExpanded
			}
		}
		m.lastCtrlOTime = time.Time{}
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	m.lastCtrlOTime = now
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].toolResult != nil {
			m.messages[i].expanded = !m.messages[i].expanded
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
		if len(m.messages[i].toolCalls) > 0 {
			m.messages[i].toolCallsExpanded = !m.messages[i].toolCallsExpanded
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
	}
	return m, nil
}

func (m *model) handleStreamCancel() (tea.Model, tea.Cmd) {
	m.cancelFunc()
	m.streaming = false
	m.streamChan = nil
	m.cancelFunc = nil
	m.buildingToolName = ""

	if m.pendingToolCalls != nil {
		for i := m.pendingToolIdx; i < len(m.pendingToolCalls); i++ {
			tc := m.pendingToolCalls[i]
			m.messages = append(m.messages, chatMessage{
				role:     "user",
				toolName: tc.Name,
				toolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Tool execution cancelled by user",
					IsError:    true,
				},
			})
		}
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
	} else if len(m.messages) > 0 {
		idx := len(m.messages) - 1
		lastMsg := m.messages[idx]
		if lastMsg.role == "assistant" && len(lastMsg.toolCalls) > 0 {
			for _, tc := range lastMsg.toolCalls {
				m.messages = append(m.messages, chatMessage{
					role:     "user",
					toolName: tc.Name,
					toolResult: &provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    "Tool execution cancelled by user",
						IsError:    true,
					},
				})
			}
		}
	}

	if len(m.messages) > 0 {
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "assistant" {
				if len(m.messages[i].toolCalls) == 0 {
					if m.messages[i].content == "" {
						m.messages[i].content = "[Interrupted]"
					} else {
						m.messages[i].content += " [Interrupted]"
					}
				}
				break
			}
		}
	}
	m.viewport.SetContent(m.renderMessages())
	return m, nil
}

func (m *model) handleHistoryUp() (tea.Model, tea.Cmd) {
	if len(m.inputHistory) == 0 {
		return m, nil
	}
	if m.historyIndex == -1 {
		m.tempInput = m.textarea.Value()
		m.historyIndex = len(m.inputHistory) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.textarea.SetValue(m.inputHistory[m.historyIndex])
	m.textarea.CursorEnd()
	m.updateTextareaHeight()
	return m, nil
}

func (m *model) handleHistoryDown() (tea.Model, tea.Cmd) {
	if m.historyIndex == -1 {
		return m, nil
	}
	if m.historyIndex < len(m.inputHistory)-1 {
		m.historyIndex++
		m.textarea.SetValue(m.inputHistory[m.historyIndex])
	} else {
		m.historyIndex = -1
		m.textarea.SetValue(m.tempInput)
	}
	m.textarea.CursorEnd()
	m.updateTextareaHeight()
	return m, nil
}

func (m *model) handleSubmit() (tea.Model, tea.Cmd) {
	if m.streaming {
		return m, nil
	}
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}

	if strings.ToLower(input) == "exit" {
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		return m, tea.Quit
	}

	m.inputHistory = append(m.inputHistory, input)
	m.historyIndex = -1
	m.tempInput = ""

	if result, isCmd := ExecuteCommand(context.Background(), m, input); isCmd {
		m.textarea.Reset()
		m.textarea.SetHeight(minTextareaHeight)
		if result != "" {
			m.messages = append(m.messages, chatMessage{role: "user", content: input})
			m.messages = append(m.messages, chatMessage{role: "system", content: result})
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, nil
		}
		// Check if this was a skill command (empty result with pending args)
		if m.pendingSkillArgs != "" {
			return m.handleSkillInvocation()
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	m.todoPanel.Clear()

	m.messages = append(m.messages, chatMessage{role: "user", content: input})
	m.textarea.Reset()
	m.textarea.SetHeight(minTextareaHeight)

	if m.llmProvider == nil {
		m.messages = append(m.messages, chatMessage{role: "system", content: "No provider connected. Use /provider to connect."})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	m.streaming = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	providerMsgs := m.convertMessagesToProvider()

	m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	modelID := m.getModelID()

	// Build extra context for system prompt
	var extra []string

	// Add available skills metadata for active skills (progressive loading)
	if skill.DefaultRegistry != nil {
		if metadata := skill.DefaultRegistry.GetAvailableSkillsPrompt(); metadata != "" {
			extra = append(extra, metadata)
		}
	}

	sysPrompt := system.Prompt(system.Config{
		Provider: m.llmProvider.Name(),
		Model:    modelID,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Extra:    extra,
	})

	tools := m.getToolsForMode()

	m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
		Model:        modelID,
		Messages:     providerMsgs,
		MaxTokens:    defaultMaxTokens,
		Tools:        tools,
		SystemPrompt: sysPrompt,
	})
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
}

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	if !m.ready {
		m.viewport = newViewport(msg.Width, msg.Height-5)
		m.viewport.SetContent(m.renderWelcome())
		m.ready = true
	} else {
		m.viewport.Width = msg.Width
	}
	m.updateViewportHeight()
	m.textarea.SetWidth(msg.Width - 4 - 2)

	m.mdRenderer = createMarkdownRenderer(msg.Width)

	return m, nil
}

// handleSkillInvocation handles skill command invocation by sending the skill
// instructions and args to the LLM.
func (m *model) handleSkillInvocation() (tea.Model, tea.Cmd) {
	if m.llmProvider == nil {
		m.messages = append(m.messages, chatMessage{role: "system", content: "No provider connected. Use /provider to connect."})
		m.pendingSkillInstructions = ""
		m.pendingSkillArgs = ""
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Get the user message (skill args or skill name)
	userMessage := m.pendingSkillArgs
	if userMessage == "" {
		userMessage = "Execute the skill."
	}

	m.todoPanel.Clear()
	m.messages = append(m.messages, chatMessage{role: "user", content: userMessage})
	m.streaming = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	providerMsgs := m.convertMessagesToProvider()

	m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	modelID := m.getModelID()

	// Build extra context for system prompt
	var extra []string

	// Add available skills metadata for active skills
	if skill.DefaultRegistry != nil {
		if metadata := skill.DefaultRegistry.GetAvailableSkillsPrompt(); metadata != "" {
			extra = append(extra, metadata)
		}
	}

	// Add full skill instructions for this invocation
	if m.pendingSkillInstructions != "" {
		extra = append(extra, m.pendingSkillInstructions)
		m.pendingSkillInstructions = ""
	}

	// Clear pending skill args
	m.pendingSkillArgs = ""

	sysPrompt := system.Prompt(system.Config{
		Provider: m.llmProvider.Name(),
		Model:    modelID,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Extra:    extra,
	})

	tools := m.getToolsForMode()

	m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
		Model:        modelID,
		Messages:     providerMsgs,
		MaxTokens:    defaultMaxTokens,
		Tools:        tools,
		SystemPrompt: sysPrompt,
	})
	return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
}

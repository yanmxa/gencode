package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/ui/history"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

const doubleTapThreshold = 500 * time.Millisecond

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Active modal/overlay components take priority
	if active, cmd := m.delegateToActiveModal(msg); active {
		return cmd, true
	}

	// Overlay modes: image selection, autocomplete suggestions
	if c, ok := m.handleImageSelectKey(msg); ok {
		return c, ok
	}
	if c, ok := m.handleSuggestionKey(msg); ok {
		return c, ok
	}

	// General input keys
	return m.handleInputKey(msg)
}

// handleImageSelectKey handles keys while in image selection mode.
func (m *model) handleImageSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.input.Images.SelectMode || len(m.input.Images.Pending) == 0 {
		return nil, false
	}
	switch msg.Type {
	case tea.KeyLeft:
		if m.input.Images.SelectedIdx > 0 {
			m.input.Images.SelectedIdx--
		}
		return nil, true
	case tea.KeyRight:
		if m.input.Images.SelectedIdx < len(m.input.Images.Pending)-1 {
			m.input.Images.SelectedIdx++
		}
		return nil, true
	case tea.KeyDelete, tea.KeyBackspace:
		m.input.Images.RemoveAt(m.input.Images.SelectedIdx)
		return nil, true
	case tea.KeyEsc:
		m.input.Images.SelectMode = false
		return nil, true
	}
	return nil, false
}

// handleSuggestionKey handles keys while the autocomplete suggestion list is visible.
func (m *model) handleSuggestionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.input.Suggestions.IsVisible() {
		return nil, false
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		m.input.Suggestions.MoveUp()
		return nil, true
	case tea.KeyDown, tea.KeyCtrlN:
		m.input.Suggestions.MoveDown()
		return nil, true
	case tea.KeyTab, tea.KeyEnter:
		if selected := m.input.Suggestions.GetSelected(); selected != "" {
			if m.input.Suggestions.GetSuggestionType() == suggest.TypeFile {
				currentValue := m.input.Textarea.Value()
				if atIdx := strings.LastIndex(currentValue, "@"); atIdx >= 0 {
					newValue := currentValue[:atIdx] + "@" + selected
					m.input.Textarea.SetValue(newValue)
					m.input.Textarea.CursorEnd()
				}
			} else {
				m.input.Textarea.SetValue(selected + " ")
				m.input.Textarea.CursorEnd()
			}
			m.input.Suggestions.Hide()
		}
		return nil, true
	case tea.KeyEsc:
		m.input.Suggestions.Hide()
		return nil, true
	}
	return nil, false
}

// handleInputKey handles general input keys (shortcuts, navigation, submit).
func (m *model) handleInputKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyShiftTab:
		if !m.conv.Stream.Active && !m.approval.IsActive() &&
			!m.mode.Question.IsActive() &&
			(m.mode.PlanApproval == nil || !m.mode.PlanApproval.IsActive()) &&
			!m.provider.Selector.IsActive() && !m.input.Suggestions.IsVisible() {
			m.cycleOperationMode()
			return nil, true
		}

	case tea.KeyCtrlO:
		return m.handleCtrlO(), true

	case tea.KeyCtrlX:
		if len(m.input.Images.Pending) > 0 {
			if m.input.Images.SelectMode {
				m.input.Images.RemoveAt(m.input.Images.SelectedIdx)
			} else {
				m.input.Images.RemoveAt(len(m.input.Images.Pending) - 1)
			}
			return nil, true
		}
		return nil, false // let textarea handle it

	case tea.KeyCtrlV, tea.KeyCtrlY:
		return m.pasteImageFromClipboard()

	case tea.KeyCtrlC:
		if m.input.Textarea.Value() != "" {
			m.input.Textarea.Reset()
			m.input.Textarea.SetHeight(appinput.MinTextareaHeight())
			m.input.HistoryIdx = -1
			return nil, true
		}
		if m.conv.Stream.Cancel != nil {
			m.conv.Stream.Cancel()
		}
		return tea.Quit, true

	case tea.KeyEsc:
		if m.input.Suggestions.IsVisible() {
			m.input.Suggestions.Hide()
			return nil, true
		}
		if m.conv.Stream.Active && m.conv.Stream.Cancel != nil {
			return m.handleStreamCancel(), true
		}
		return nil, true

	case tea.KeyUp:
		if m.input.Textarea.Line() == 0 {
			if len(m.input.Images.Pending) > 0 && !m.input.Images.SelectMode {
				m.input.Images.SelectMode = true
				m.input.Images.SelectedIdx = len(m.input.Images.Pending) - 1
				return nil, true
			}
			return m.handleHistoryUp(), true
		}

	case tea.KeyDown:
		lines := strings.Count(m.input.Textarea.Value(), "\n")
		if m.input.Textarea.Line() == lines {
			return m.handleHistoryDown(), true
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.input.Textarea.InsertString("\n")
			m.input.UpdateHeight()
			return nil, true
		}
		return m.handleSubmit(), true
	}

	// Not handled, let textarea handle it
	return nil, false
}

// delegateToActiveModal routes keypresses to active modal components.
// For prompts, responses are handled directly here instead of routing through the message loop.
// Returns (true, cmd) if a modal is active and handled the keypress, (false, nil) otherwise.
func (m *model) delegateToActiveModal(msg tea.KeyMsg) (bool, tea.Cmd) {
	// Check prompts first — handle responses directly
	if m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive() {
		cmd, resp := m.mode.PlanApproval.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handlePlanResponse(*resp))
		}
		return true, cmd
	}
	if m.mode.Question.IsActive() {
		cmd, resp := m.mode.Question.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handleQuestionResponse(*resp))
		}
		return true, cmd
	}
	if m.approval.IsActive() {
		cmd, resp := m.approval.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handlePermissionResponse(*resp))
		}
		return true, cmd
	}
	if m.mode.PlanEntry.IsActive() {
		cmd, resp := m.mode.PlanEntry.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handleEnterPlanResponse(*resp))
		}
		return true, cmd
	}

	// Check selectors
	if m.provider.Selector.IsActive() {
		return true, m.provider.Selector.HandleKeypress(msg)
	}
	if m.tool.Selector.IsActive() {
		return true, m.tool.Selector.HandleKeypress(msg)
	}
	if m.skill.Selector.IsActive() {
		return true, m.skill.Selector.HandleKeypress(msg)
	}
	if m.agent.Selector.IsActive() {
		return true, m.agent.Selector.HandleKeypress(msg)
	}
	if m.mcp.Selector.IsActive() {
		return true, m.mcp.Selector.HandleKeypress(msg)
	}
	if m.plugin.Selector.IsActive() {
		return true, m.plugin.Selector.HandleKeypress(msg)
	}
	if m.session.Selector.IsActive() {
		return true, m.session.Selector.HandleKeypress(msg)
	}
	if m.memory.Selector.IsActive() {
		return true, m.memory.Selector.HandleKeypress(msg)
	}

	return false, nil
}

func (m *model) handleCtrlO() tea.Cmd {
	// Handle permission prompt preview toggle
	if m.approval != nil && m.approval.IsActive() {
		m.togglePermissionPreview()
		return nil
	}

	now := time.Now()
	if now.Sub(m.input.LastCtrlO) < doubleTapThreshold {
		// Double-tap: toggle all uncommitted expandable items
		anyExpanded := false
		for i := m.conv.CommittedCount; i < len(m.conv.Messages); i++ {
			msg := m.conv.Messages[i]
			if (msg.ToolResult != nil && msg.Expanded) ||
				(len(msg.ToolCalls) > 0 && msg.ToolCallsExpanded) {
				anyExpanded = true
				break
			}
		}
		for i := m.conv.CommittedCount; i < len(m.conv.Messages); i++ {
			if m.conv.Messages[i].ToolResult != nil {
				m.conv.Messages[i].Expanded = !anyExpanded
			}
			if len(m.conv.Messages[i].ToolCalls) > 0 {
				m.conv.Messages[i].ToolCallsExpanded = !anyExpanded
			}
		}
		m.input.LastCtrlO = time.Time{}
		return nil
	}

	// Single tap: toggle most recent expandable item
	m.input.LastCtrlO = now
	m.conv.ToggleMostRecentExpandable()
	return nil
}

func (m *model) handleStreamCancel() tea.Cmd {
	m.conv.Stream.Cancel()
	m.conv.Stream.Stop()

	// Cancel pending tool calls
	m.cancelPendingToolCalls()

	// Mark the last assistant message as interrupted
	m.conv.MarkLastInterrupted()

	// Commit all messages to scrollback
	return tea.Batch(m.commitMessages()...)
}

// cancelPendingToolCalls adds cancellation messages for pending tool calls.
func (m *model) cancelPendingToolCalls() {
	var toolCalls []message.ToolCall

	if m.tool.PendingCalls != nil {
		toolCalls = m.tool.PendingCalls[m.tool.CurrentIdx:]
		m.tool.PendingCalls = nil
		m.tool.CurrentIdx = 0
	} else if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		if lastMsg.Role == message.RoleAssistant {
			toolCalls = lastMsg.ToolCalls
		}
	}

	for _, tc := range toolCalls {
		m.conv.Append(message.ChatMessage{
			Role:     message.RoleUser,
			ToolName: tc.Name,
			ToolResult: &message.ToolResult{
				ToolCallID: tc.ID,
				Content:    "Tool execution cancelled by user",
				IsError:    true,
			},
		})
	}
}

func (m *model) handleHistoryUp() tea.Cmd {
	m.input.HistoryUp()
	return nil
}

func (m *model) handleHistoryDown() tea.Cmd {
	m.input.HistoryDown()
	return nil
}

func (m *model) handleSubmit() tea.Cmd {
	if m.conv.Stream.Active {
		return nil
	}
	input := strings.TrimSpace(m.input.Textarea.Value())
	if input == "" && len(m.input.Images.Pending) == 0 {
		return nil
	}

	if strings.ToLower(input) == "exit" {
		if m.conv.Stream.Cancel != nil {
			m.conv.Stream.Cancel()
		}
		return tea.Quit
	}

	// Execute UserPromptSubmit hook before processing
	if blocked, reason := m.checkPromptHook(input); blocked {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Prompt blocked: " + reason})
		m.input.Textarea.Reset()
		m.input.Textarea.SetHeight(appinput.MinTextareaHeight())
		return tea.Batch(m.commitMessages()...)
	}

	if input != "" {
		m.input.History = append(m.input.History, input)
		m.input.HistoryIdx = -1
		m.input.TempInput = ""
		history.Save(m.cwd, m.input.History)
	}

	if result, cmd, isCmd := ExecuteCommand(context.Background(), m, input); isCmd {
		m.input.Textarea.Reset()
		m.input.Textarea.SetHeight(appinput.MinTextareaHeight())

		if result != "" {
			m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: input})
			m.conv.AddNotice(result)
		}

		cmds := m.commitMessages()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return tea.Batch(cmds...)
	}

	// Process @image.png references
	content, fileImages, err := appinput.ProcessImageRefs(m.cwd, input)
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Image error: " + err.Error()})
		return tea.Batch(m.commitMessages()...)
	}

	// Combine pending clipboard images with file reference images
	allImages := append(m.input.Images.Pending, fileImages...)
	m.input.Images.Pending = nil // Clear pending images

	m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: content, Images: allImages})
	m.input.Textarea.Reset()
	m.input.Textarea.SetHeight(appinput.MinTextareaHeight())

	if m.provider.LLM == nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "No provider connected. Use /provider to connect."})
		return tea.Batch(m.commitMessages()...)
	}

	return m.startLLMStream(nil)
}

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) tea.Cmd {
	m.width = msg.Width
	m.height = msg.Height

	// Update markdown renderer before rendering any content
	m.output.ResizeMDRenderer(msg.Width)

	// Resize plan prompt if active
	if m.mode.PlanApproval != nil {
		m.mode.PlanApproval.SetSize(msg.Width, msg.Height)
	}

	if !m.ready {
		m.ready = true

		var cmds []tea.Cmd

		// If resuming a session with messages, commit them to scrollback
		if len(m.conv.Messages) > 0 {
			cmds = append(cmds, m.commitAllMessages()...)
		} else {
			// Print welcome screen
			cmds = append(cmds, tea.Println(m.renderWelcome()))
		}

		// Open session selector if pending (for --resume flag)
		if m.session.PendingSelector {
			m.session.PendingSelector = false
			if m.session.Store != nil {
				_ = m.session.Selector.EnterSelect(m.width, m.height, m.session.Store, m.cwd)
			}
		}

		m.input.Textarea.SetWidth(msg.Width - 4 - 2)
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
		return nil
	}

	m.input.Textarea.SetWidth(msg.Width - 4 - 2)
	return nil
}

// handleSkillInvocation handles skill command invocation by sending the skill
// instructions and args to the LLM.
func (m *model) handleSkillInvocation() tea.Cmd {
	if m.provider.LLM == nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "No provider connected. Use /provider to connect."})
		m.skill.PendingInstructions = ""
		m.skill.PendingArgs = ""
		return tea.Batch(m.commitMessages()...)
	}

	// Get the user message (skill args or skill name)
	userMessage := m.skill.PendingArgs
	if userMessage == "" {
		userMessage = "Execute the skill."
	}

	m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: userMessage})

	// Build extra with skill instructions
	var extra []string
	if m.skill.PendingInstructions != "" {
		extra = append(extra, m.skill.PendingInstructions)
		m.skill.PendingInstructions = ""
	}
	m.skill.PendingArgs = ""

	return m.startLLMStream(extra)
}

// checkPromptHook runs UserPromptSubmit hook and returns (blocked, reason).
func (m *model) checkPromptHook(prompt string) (bool, string) {
	if m.hookEngine == nil {
		return false, ""
	}
	outcome := m.hookEngine.Execute(context.Background(), hooks.UserPromptSubmit, hooks.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

// pasteImageFromClipboard handles pasting image from clipboard
func (m *model) pasteImageFromClipboard() (tea.Cmd, bool) {
	imgData, err := image.ReadImageToProviderData()
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Image paste error: " + err.Error()})
		return tea.Batch(m.commitMessages()...), true
	}
	if imgData == nil {
		// No image in clipboard, let textarea handle the key
		return nil, false
	}
	m.input.Images.Pending = append(m.input.Images.Pending, *imgData)
	return nil, true
}

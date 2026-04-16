package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hook"
)

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Active modal/overlay components take priority
	if active, cmd := m.delegateToActiveModal(msg); active {
		return cmd, true
	}

	// Overlay modes: image selection, autocomplete suggestions, queue selection
	if c, ok := m.userInput.HandleImageSelectKey(msg); ok {
		return c, ok
	}
	if c, ok := m.userInput.HandleSuggestionKey(msg); ok {
		return c, ok
	}
	if c, ok := m.handleQueueSelectKey(msg); ok {
		return c, ok
	}

	// General input keys
	return m.handleInputKey(msg)
}

// handleInputKey handles general input keys (shortcuts, navigation, submit).
func (m *model) handleInputKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyRight:
		// Accept ghost text suggestion
		if m.promptSuggestion.text != "" && m.userInput.Textarea.Value() == "" {
			m.userInput.Textarea.SetValue(m.promptSuggestion.text)
			m.userInput.Textarea.CursorEnd()
			m.promptSuggestion.Clear()
			return nil, true
		}

	case tea.KeyShiftTab:
		if !m.conv.Stream.Active && !m.approval.IsActive() &&
			!m.mode.Question.IsActive() &&
			(m.mode.PlanApproval == nil || !m.mode.PlanApproval.IsActive()) &&
			!m.provider.Selector.IsActive() && !m.userInput.Suggestions.IsVisible() {
			m.cycleOperationMode()
			return nil, true
		}

	case tea.KeyCtrlT:
		m.showTasks = !m.showTasks
		return nil, true

	case tea.KeyCtrlO:
		return m.handleCtrlO(), true

	case tea.KeyCtrlE:
		return m.expandCollapseAll(), true

	case tea.KeyCtrlX:
		return nil, false // let textarea handle it

	case tea.KeyCtrlU:
		if m.inputQueue.Len() > 0 {
			m.inputQueue.Clear()
			m.queueSelectIdx = -1
			m.queueTempInput = ""
			return nil, true
		}
		return nil, false // let textarea handle (clear to start of line)

	case tea.KeyCtrlV, tea.KeyCtrlY:
		return m.pasteImageFromClipboard()

	case tea.KeyCtrlC:
		if m.userInput.Textarea.Value() != "" {
			m.resetInputField()
			m.userInput.HistoryIdx = -1
			return nil, true
		}
		return m.quitWithCancel()

	case tea.KeyCtrlD:
		if m.userInput.Textarea.Value() != "" {
			return nil, false // let textarea handle deletion
		}
		return m.quitWithCancel()

	case tea.KeyCtrlL:
		_, cmd, _ := handleClearCommand(context.Background(), m, "")
		return cmd, true

	case tea.KeyEsc:
		if m.promptSuggestion.text != "" {
			m.promptSuggestion.Clear()
			return nil, true
		}
		if m.userInput.Suggestions.IsVisible() {
			m.userInput.Suggestions.Hide()
			return nil, true
		}
		if m.conv.Stream.Active {
			return m.handleStreamCancel(), true
		}
		return nil, true

	case tea.KeyUp:
		if m.userInput.Textarea.Line() == 0 {
			// Queue items are displayed above the input — Up goes there first
			if m.inputQueue.Len() > 0 {
				m.enterQueueSelection()
				return nil, true
			}
			return m.handleHistoryUp(), true
		}

	case tea.KeyDown:
		lines := strings.Count(m.userInput.Textarea.Value(), "\n")
		if m.userInput.Textarea.Line() == lines {
			return m.handleHistoryDown(), true
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.userInput.Textarea.InsertString("\n")
			m.userInput.UpdateHeight()
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

	switch {
	case m.provider.Selector.IsActive():
		return true, m.provider.Selector.HandleKeypress(msg)
	case m.tool.Selector.IsActive():
		return true, m.tool.Selector.HandleKeypress(msg)
	case m.skill.Selector.IsActive():
		return true, m.skill.Selector.HandleKeypress(msg)
	case m.agent.Selector.IsActive():
		return true, m.agent.Selector.HandleKeypress(msg)
	case m.mcp.Selector.IsActive():
		return true, m.mcp.Selector.HandleKeypress(msg)
	case m.plugin.Selector.IsActive():
		return true, m.plugin.Selector.HandleKeypress(msg)
	case m.session.Selector.IsActive():
		return true, m.session.Selector.HandleKeypress(msg)
	case m.memory.Selector.IsActive():
		return true, m.memory.Selector.HandleKeypress(msg)
	case m.search.Selector.IsActive():
		return true, m.search.Selector.HandleKeypress(msg)
	}

	return false, nil
}

const ctrlODoubleTapWindow = 300 * time.Millisecond

// ctrlOSingleTickMsg fires after the double-tap window expires,
// indicating the user intended a single ctrl+o press.
type ctrlOSingleTickMsg struct{}

func (m *model) handleCtrlO() tea.Cmd {
	// Handle permission prompt preview toggle
	if m.approval != nil && m.approval.IsActive() {
		m.togglePermissionPreview()
		return nil
	}

	now := time.Now()
	if !m.userInput.LastCtrlO.IsZero() && now.Sub(m.userInput.LastCtrlO) < ctrlODoubleTapWindow {
		// Double-tap: toggle all expandable items
		m.userInput.LastCtrlO = time.Time{}
		return m.expandCollapseAll()
	}

	// First tap: wait to see if a second tap follows
	m.userInput.LastCtrlO = now
	return tea.Tick(ctrlODoubleTapWindow, func(time.Time) tea.Msg {
		return ctrlOSingleTickMsg{}
	})
}

// handleCtrlOSingleTick fires when the double-tap window expires
// without a second press — execute single toggle.
// Returns a tea.Cmd to reflow scrollback (nil if no-op).
func (m *model) handleCtrlOSingleTick() tea.Cmd {
	if m.userInput.LastCtrlO.IsZero() {
		return nil // already consumed by double-tap
	}
	m.userInput.LastCtrlO = time.Time{}
	m.conv.ToggleMostRecentExpandable()
	// Re-render via reflowScrollback so content stays in terminal scrollback
	return m.reflowScrollback()
}

// expandCollapseAll toggles expand/collapse for all tool results and tool calls.
func (m *model) expandCollapseAll() tea.Cmd {
	anyExpanded := false
	for i := 0; i < len(m.conv.Messages); i++ {
		msg := m.conv.Messages[i]
		if (msg.ToolResult != nil && msg.Expanded) ||
			(len(msg.ToolCalls) > 0 && msg.ToolCallsExpanded) {
			anyExpanded = true
			break
		}
	}
	for i := 0; i < len(m.conv.Messages); i++ {
		if m.conv.Messages[i].ToolResult != nil {
			m.conv.Messages[i].Expanded = !anyExpanded
		}
		if len(m.conv.Messages[i].ToolCalls) > 0 {
			m.conv.Messages[i].ToolCallsExpanded = !anyExpanded
		}
	}
	// Re-render committed messages with new expand state via scrollback
	return m.reflowScrollback()
}

func (m *model) handleHistoryUp() tea.Cmd {
	m.userInput.HistoryUp()
	return nil
}

func (m *model) handleHistoryDown() tea.Cmd {
	m.userInput.HistoryDown()
	return nil
}

// checkPromptHook runs UserPromptSubmit hook and returns (blocked, reason).
func (m *model) checkPromptHook(prompt string) (bool, string) {
	if m.hookEngine == nil {
		return false, ""
	}
	outcome := m.hookEngine.Execute(context.Background(), hook.UserPromptSubmit, hook.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

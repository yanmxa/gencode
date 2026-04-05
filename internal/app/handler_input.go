package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

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
	case tea.KeyTab:
		// Accept ghost text suggestion
		if m.promptSuggestion.text != "" && m.input.Textarea.Value() == "" {
			m.input.Textarea.SetValue(m.promptSuggestion.text)
			m.input.Textarea.CursorEnd()
			m.promptSuggestion.Clear()
			return nil, true
		}

	case tea.KeyShiftTab:
		if !m.conv.Stream.Active && !m.approval.IsActive() &&
			!m.mode.Question.IsActive() &&
			(m.mode.PlanApproval == nil || !m.mode.PlanApproval.IsActive()) &&
			!m.provider.Selector.IsActive() && !m.input.Suggestions.IsVisible() {
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
		return m.quitWithCancel()

	case tea.KeyCtrlD:
		if m.input.Textarea.Value() != "" {
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

	// Check selectors via unified interface dispatch.
	for _, sel := range m.overlaySelectors() {
		if sel.IsActive() {
			return true, sel.HandleKeypress(msg)
		}
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
	if !m.input.LastCtrlO.IsZero() && now.Sub(m.input.LastCtrlO) < ctrlODoubleTapWindow {
		// Double-tap: toggle all expandable items
		m.input.LastCtrlO = time.Time{}
		return m.expandCollapseAll()
	}

	// First tap: wait to see if a second tap follows
	m.input.LastCtrlO = now
	return tea.Tick(ctrlODoubleTapWindow, func(time.Time) tea.Msg {
		return ctrlOSingleTickMsg{}
	})
}

// handleCtrlOSingleTick fires when the double-tap window expires
// without a second press — execute single toggle.
// Returns a tea.Cmd to reflow scrollback (nil if no-op).
func (m *model) handleCtrlOSingleTick() tea.Cmd {
	if m.input.LastCtrlO.IsZero() {
		return nil // already consumed by double-tap
	}
	m.input.LastCtrlO = time.Time{}
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
	m.input.HistoryUp()
	return nil
}

func (m *model) handleHistoryDown() tea.Cmd {
	m.input.HistoryDown()
	return nil
}

// checkPromptHook runs UserPromptSubmit hook and returns (blocked, reason).
func (m *model) checkPromptHook(prompt string) (bool, string) {
	if m.hookEngine == nil {
		return false, ""
	}
	outcome := m.hookEngine.Execute(context.Background(), hooks.UserPromptSubmit, hooks.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

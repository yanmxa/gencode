package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit/suggest"
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
		if !m.conv.Stream.Active && !m.userInput.Approval.IsActive() &&
			!m.mode.Question.IsActive() &&
			(m.mode.PlanApproval == nil || !m.mode.PlanApproval.IsActive()) &&
			!m.userInput.Provider.Selector.IsActive() && !m.userInput.Suggestions.IsVisible() {
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
		if m.userInput.Queue.Len() > 0 {
			m.userInput.Queue.Clear()
			m.userInput.QueueSelectIdx = -1
			m.userInput.QueueTempInput = ""
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
			if m.userInput.Queue.Len() > 0 {
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
	if m.userInput.Approval.IsActive() {
		cmd, resp := m.userInput.Approval.HandleKeypress(msg)
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

	// Delegate to overlay selectors via the shared overlaySelectors() list
	for _, s := range m.overlaySelectors() {
		if s.IsActive() {
			return true, s.HandleKeypress(msg)
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
	if m.userInput.Approval.IsActive() {
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
	if m.runtime.HookEngine == nil {
		return false, ""
	}
	outcome := m.runtime.HookEngine.Execute(context.Background(), hook.UserPromptSubmit, hook.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

// --- Queue selection (Source 1 overlay) ---

func (m *model) handleQueueSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	return m.userInput.HandleQueueSelectKey(msg)
}

func (m *model) enterQueueSelection() {
	m.userInput.EnterQueueSelection()
}

func (m *model) saveCurrentQueueEdit() {
	m.userInput.SaveCurrentQueueEdit()
}

// --- Prompt suggestion (ghost text) ---

type promptSuggestionMsg struct {
	text string
	err  error
}

type promptSuggestionState struct {
	text   string
	cancel context.CancelFunc
}

func (s *promptSuggestionState) Clear() {
	s.text = ""
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

const suggestionSystemPrompt = `You predict what the user will type next in a coding assistant CLI.
Reply with ONLY the predicted text (2-12 words). No quotes, no explanation.
If unsure, reply with nothing.`

const suggestionUserPrompt = `[PREDICTION MODE] Based on this conversation, predict what the user will type next.
Stay silent if the next step isn't obvious. Match the user's language and style.`

const maxSuggestionMessages = 20

func (m *model) startPromptSuggestion() tea.Cmd {
	req, ok := m.buildPromptSuggestionRequest()
	if !ok {
		return nil
	}

	m.promptSuggestion.Clear()

	ctx, cancel := context.WithCancel(context.Background())
	m.promptSuggestion.cancel = cancel
	req.Ctx = ctx

	return suggestPromptCmd(req)
}

func (m *model) handlePromptSuggestion(msg promptSuggestionMsg) {
	if msg.err != nil {
		return
	}
	if m.userInput.Textarea.Value() != "" {
		return
	}
	if m.conv.Stream.Active {
		return
	}
	if text := suggest.FilterSuggestion(msg.text); text != "" {
		m.promptSuggestion.text = text
	}
}

// --- Window resize ---

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) tea.Cmd {
	oldWidth := m.width
	m.width = msg.Width
	m.height = msg.Height
	m.userInput.TerminalHeight = msg.Height

	m.agentOutput.ResizeMDRenderer(msg.Width)

	if m.mode.PlanApproval != nil {
		m.mode.PlanApproval.SetSize(msg.Width, msg.Height)
	}

	if !m.ready {
		m.ready = true

		var cmds []tea.Cmd
		if len(m.conv.Messages) > 0 {
			cmds = append(cmds, m.commitAllMessages()...)
		} else {
			cmds = append(cmds, tea.Println(m.renderWelcome()))
		}

		if m.userInput.Session.PendingSelector {
			m.userInput.Session.PendingSelector = false
			if m.runtime.SessionStore != nil {
				_ = m.userInput.Session.Selector.EnterSelect(m.width, m.height, m.runtime.SessionStore, m.cwd)
			}
		}

		m.userInput.Textarea.SetWidth(msg.Width - 4 - 2)
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
		return nil
	}

	m.userInput.Textarea.SetWidth(msg.Width - 4 - 2)

	if oldWidth != msg.Width && m.conv.CommittedCount > 0 {
		return m.reflowScrollback()
	}

	return nil
}

func (m *model) reflowScrollback() tea.Cmd {
	committed := m.conv.CommittedCount
	m.conv.CommittedCount = 0

	var cmds []tea.Cmd
	cmds = append(cmds, tea.ClearScreen)

	for i := 0; i < committed; i++ {
		if rendered := m.renderSingleMessage(i); rendered != "" {
			cmds = append(cmds, tea.Println(rendered))
		}
		m.conv.CommittedCount = i + 1
	}

	return tea.Sequence(cmds...)
}

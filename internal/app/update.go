// Bubble Tea Update: top-level message dispatch, key routing, and resize handling.
package app

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/skill"
)

// --- Routing types & helpers ---

type messageUpdater func(*model, tea.Msg) (tea.Cmd, bool)

// overlaySelector is implemented by full-screen selector components that can
// render themselves and receive keyboard input when active.
type overlaySelector interface {
	IsActive() bool
	HandleKeypress(tea.KeyMsg) tea.Cmd
	Render() string
}

func (m *model) overlaySelectors() []overlaySelector {
	return []overlaySelector{
		&m.userInput.Provider.Selector,
		&m.tool.Selector,
		&m.userInput.Skill.Selector,
		&m.userInput.Agent,
		&m.userInput.MCP.Selector,
		&m.userInput.Plugin,
		&m.userInput.Session.Selector,
		&m.userInput.Memory.Selector,
		&m.userInput.Search,
	}
}

// initialPromptMsg is sent from Init() to inject an initial CLI prompt.
type initialPromptMsg string

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Input & UI chrome ────────────────────────────────────
	switch msg := msg.(type) {
	case initialPromptMsg:
		m.userInput.Textarea.SetValue(string(msg))
		return m, m.handleSubmit()
	case tea.KeyMsg:
		if c, ok := m.handleKeypress(msg); ok {
			return m, c
		}
	case tea.WindowSizeMsg:
		return m, m.handleWindowResize(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		return m, cmd
	case appuser.SkillInvokeMsg:
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(m, sk, "")
			return m, m.handleSkillInvocation()
		}
		return m, nil
	case ctrlOSingleTickMsg:
		return m, m.handleCtrlOSingleTick()
	case promptSuggestionMsg:
		m.handlePromptSuggestion(msg)
		return m, nil
	case kit.DismissedMsg, appoutput.ToggleMsg, appuser.SkillCycleMsg, appuser.AgentToggleMsg:
		return m, nil
	}

	// ── Feature routing ──────────────────────────────────────
	if cmd, handled := m.routeFeatureUpdate(msg); handled {
		return m, cmd
	}
	// ── Fallthrough: forward to textarea & spinner ────────────
	return m, m.updateTextarea(msg)
}

// --- Feature routing ---

func (m *model) routeFeatureUpdate(msg tea.Msg) (tea.Cmd, bool) {
	for _, updater := range [...]messageUpdater{
		(*model).updateOutput, // agent outbox, perm bridge, compact results
		(*model).updateAgentInput,
		(*model).updateApproval,
		(*model).updateMode,
		(*model).updateUserOverlays, // provider, MCP, plugin, session, memory, search
		(*model).updateSystemInput,
	} {
		if cmd, handled := updater(m, msg); handled {
			return cmd, true
		}
	}
	return nil, false
}

func (m *model) renderOverlaySelector() string {
	for _, s := range m.overlaySelectors() {
		if s.IsActive() {
			return s.Render()
		}
	}
	return ""
}

func (m *model) renderActiveModal(separator, trackerPrefix string) string {
	switch {
	case m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.mode.PlanApproval.RenderMenu())
	case m.userInput.Approval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.userInput.Approval.Render())
	case m.mode.Question.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.mode.Question.Render())
	case m.mode.PlanEntry.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.mode.PlanEntry.Render())
	default:
		return ""
	}
}

func separatorWrapped(trackerPrefix, separator, content string) string {
	return trackerPrefix + separator + "\n" + content
}

// updateTextarea forwards unhandled messages to the textarea and spinner.
func (m *model) updateTextarea(msg tea.Msg) tea.Cmd {
	cmd, changed := m.userInput.HandleTextareaUpdate(msg)
	cmds := []tea.Cmd{cmd}
	if changed {
		m.promptSuggestion.Clear()
	}

	if m.conv.Stream.Active || m.userInput.Provider.FetchingLimits || m.conv.Compact.Active {
		var spinnerCmd tea.Cmd
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		spinnerCmd = cmd
		cmds = append(cmds, spinnerCmd)
	}

	return tea.Batch(cmds...)
}

// --- Key dispatch ---

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Cmd, bool) {
	if active, cmd := m.delegateToActiveModal(msg); active {
		return cmd, true
	}

	if c, ok := m.userInput.HandleImageSelectKey(msg); ok {
		return c, ok
	}
	if c, ok := m.userInput.HandleSuggestionKey(msg); ok {
		return c, ok
	}
	if c, ok := m.userInput.HandleQueueSelectKey(msg); ok {
		return c, ok
	}

	return m.handleInputKey(msg)
}

// handleInputKey handles general input keys (shortcuts, navigation, submit).
func (m *model) handleInputKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyRight:
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
			m.userInput.Reset()
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
			if m.userInput.Queue.Len() > 0 {
				m.userInput.EnterQueueSelection()
				return nil, true
			}
			m.userInput.HistoryUp()
			return nil, true
		}

	case tea.KeyDown:
		lines := strings.Count(m.userInput.Textarea.Value(), "\n")
		if m.userInput.Textarea.Line() == lines {
			m.userInput.HistoryDown()
			return nil, true
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.userInput.Textarea.InsertString("\n")
			m.userInput.UpdateHeight()
			return nil, true
		}
		return m.handleSubmit(), true

	}

	return nil, false
}

// delegateToActiveModal routes keypresses to active modal components.
func (m *model) delegateToActiveModal(msg tea.KeyMsg) (bool, tea.Cmd) {
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

	for _, s := range m.overlaySelectors() {
		if s.IsActive() {
			return true, s.HandleKeypress(msg)
		}
	}

	return false, nil
}

// --- Ctrl+O / expand-collapse ---

const ctrlODoubleTapWindow = 300 * time.Millisecond

type ctrlOSingleTickMsg struct{}

func (m *model) handleCtrlO() tea.Cmd {
	if m.userInput.Approval.IsActive() {
		m.togglePermissionPreview()
		return nil
	}

	now := time.Now()
	if !m.userInput.LastCtrlO.IsZero() && now.Sub(m.userInput.LastCtrlO) < ctrlODoubleTapWindow {
		m.userInput.LastCtrlO = time.Time{}
		return m.expandCollapseAll()
	}

	m.userInput.LastCtrlO = now
	return tea.Tick(ctrlODoubleTapWindow, func(time.Time) tea.Msg {
		return ctrlOSingleTickMsg{}
	})
}

func (m *model) handleCtrlOSingleTick() tea.Cmd {
	if m.userInput.LastCtrlO.IsZero() {
		return nil // already consumed by double-tap
	}
	m.userInput.LastCtrlO = time.Time{}
	m.conv.ToggleMostRecentExpandable()
	return m.reflowScrollback()
}

func (m *model) expandCollapseAll() tea.Cmd {
	m.conv.ToggleAllExpandable()
	return m.reflowScrollback()
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

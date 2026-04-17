// Bubble Tea Update: top-level message dispatch and feature routing.
package app

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
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
		&m.provider.Selector,
		&m.tool.Selector,
		&m.userInput.Skill.Selector,
		&m.userInput.Agent,
		&m.userInput.MCP.Selector,
		&m.plugin,
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
	case kit.DismissedMsg, toolui.ToggleMsg, appuser.SkillCycleMsg, appuser.AgentToggleMsg:
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
		(*model).updateProvider,
		(*model).updateMCP,
		(*model).updatePlugin,
		(*model).updateSession,
		(*model).updateMemory,
		(*model).updateSystemInput,
		(*model).updateSearch,
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

	if m.conv.Stream.Active || m.provider.FetchingLimits || m.conv.Compact.Active {
		var spinnerCmd tea.Cmd
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		spinnerCmd = cmd
		cmds = append(cmds, spinnerCmd)
	}

	return tea.Batch(cmds...)
}

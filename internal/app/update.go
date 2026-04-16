// Bubble Tea Update: top-level message dispatch and feature routing.
package app

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/user/agentui"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/user/skillui"
	"github.com/yanmxa/gencode/internal/app/output/toolui"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/extension/skill"
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
		&m.skill.Selector,
		&m.agent,
		&m.mcp.Selector,
		&m.plugin.Selector,
		&m.session.Selector,
		&m.memory.Selector,
		&m.search,
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
	case skillui.InvokeMsg:
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
	case kit.DismissedMsg, toolui.ToggleMsg, skillui.CycleMsg, agentui.ToggleMsg:
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
		(*model).updateOutput, // agent outbox -> TUI output path
		(*model).updateAgentInput,
		(*model).updateApproval,
		(*model).updateMode,
		(*model).updateCompact,
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
	switch {
	case m.provider.Selector.IsActive():
		return m.provider.Selector.Render()
	case m.tool.Selector.IsActive():
		return m.tool.Selector.Render()
	case m.skill.Selector.IsActive():
		return m.skill.Selector.Render()
	case m.agent.IsActive():
		return m.agent.Render()
	case m.mcp.Selector.IsActive():
		return m.mcp.Selector.Render()
	case m.plugin.Selector.IsActive():
		return m.plugin.Selector.Render()
	case m.session.Selector.IsActive():
		return m.session.Selector.Render()
	case m.memory.Selector.IsActive():
		return m.memory.Selector.Render()
	case m.search.IsActive():
		return m.search.Render()
	default:
		return ""
	}
}

func (m *model) renderActiveModal(separator, trackerPrefix string) string {
	switch {
	case m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.mode.PlanApproval.RenderMenu())
	case m.approval != nil && m.approval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.approval.Render())
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
	cmd, changed := appuser.HandleTextareaUpdate(&m.userInput, msg)
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

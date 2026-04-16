package app

import tea "github.com/charmbracelet/bubbletea"

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
		&m.agent.Selector,
		&m.mcp.Selector,
		&m.plugin.Selector,
		&m.session.Selector,
		&m.memory.Selector,
		&m.search.Selector,
	}
}

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
	case m.agent.Selector.IsActive():
		return m.agent.Selector.Render()
	case m.mcp.Selector.IsActive():
		return m.mcp.Selector.Render()
	case m.plugin.Selector.IsActive():
		return m.plugin.Selector.Render()
	case m.session.Selector.IsActive():
		return m.session.Selector.Render()
	case m.memory.Selector.IsActive():
		return m.memory.Selector.Render()
	case m.search.Selector.IsActive():
		return m.search.Selector.Render()
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

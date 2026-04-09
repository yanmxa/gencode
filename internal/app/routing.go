package app

import tea "github.com/charmbracelet/bubbletea"

type messageUpdater func(*model, tea.Msg) (tea.Cmd, bool)

func (m *model) featureUpdaters() []messageUpdater {
	return []messageUpdater{
		(*model).updateStream,
		(*model).updateTool,
		(*model).updateApproval,
		(*model).updateMode,
		(*model).updateCompact,
		(*model).updateProvider,
		(*model).updateMCP,
		(*model).updatePlugin,
		(*model).updateSession,
		(*model).updateMemory,
		(*model).updateCron,
		(*model).updateAsyncHooks,
		(*model).updateTaskNotifications,
	}
}

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
	}
}

func (m *model) routeFeatureUpdate(msg tea.Msg) (tea.Cmd, bool) {
	for _, updater := range m.featureUpdaters() {
		if cmd, handled := updater(m, msg); handled {
			return cmd, true
		}
	}
	return nil, false
}

func (m *model) renderOverlaySelector() string {
	for _, selector := range m.overlaySelectors() {
		if selector.IsActive() {
			return selector.Render()
		}
	}
	return ""
}

type modalRenderer struct {
	isActive func() bool
	render   func() string
}

func (m *model) modalRenderers(separator, trackerPrefix string) []modalRenderer {
	return []modalRenderer{
		{
			isActive: func() bool {
				return m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive()
			},
			render: func() string {
				return separatorWrapped(trackerPrefix, separator, m.mode.PlanApproval.RenderMenu())
			},
		},
		{
			isActive: func() bool {
				return m.approval != nil && m.approval.IsActive()
			},
			render: func() string {
				return separatorWrapped(trackerPrefix, separator, m.approval.Render())
			},
		},
		{
			isActive: func() bool {
				return m.mode.Question.IsActive()
			},
			render: func() string {
				return separatorWrapped(trackerPrefix, separator, m.mode.Question.Render())
			},
		},
		{
			isActive: func() bool {
				return m.mode.PlanEntry.IsActive()
			},
			render: func() string {
				return separatorWrapped(trackerPrefix, separator, m.mode.PlanEntry.Render())
			},
		},
	}
}

func (m *model) renderActiveModal(separator, trackerPrefix string) string {
	for _, modal := range m.modalRenderers(separator, trackerPrefix) {
		if modal.isActive() {
			return modal.render()
		}
	}
	return ""
}

func separatorWrapped(trackerPrefix, separator, content string) string {
	return trackerPrefix + separator + "\n" + content
}

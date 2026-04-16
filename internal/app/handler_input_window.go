package app

import tea "github.com/charmbracelet/bubbletea"

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

		if m.session.PendingSelector {
			m.session.PendingSelector = false
			if m.session.Store != nil {
				_ = m.session.Selector.EnterSelect(m.width, m.height, m.session.Store, m.cwd)
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

// reflowScrollback clears the terminal and re-commits all previously committed
// messages at the current width. This is needed when the pane width changes
// (e.g., after tmux split) to fix content that was rendered at the old width.
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

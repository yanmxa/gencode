package notify

import tea "github.com/charmbracelet/bubbletea"

// Deps holds the app-level state and callbacks needed to process Source 2 input.
type Deps struct {
	StreamActive bool
	Inject       func(Notification) tea.Cmd
	BGTracker    *BackgroundTracker
}

// Update routes Source 2 (agent -> agent) task-notification messages.
func Update(deps Deps, state *Model, msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(TickMsg); !ok {
		return nil, false
	}
	return handleTick(deps, state), true
}

func handleTick(deps Deps, state *Model) tea.Cmd {
	cmds := []tea.Cmd{StartTicker()}

	if deps.BGTracker != nil {
		deps.BGTracker.ResetIfIdle(deps.StreamActive)
	}

	items := PopReadyNotifications(state.Notifications, !deps.StreamActive)
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, deps.Inject(MergeNotifications(items)))
	return tea.Batch(cmds...)
}

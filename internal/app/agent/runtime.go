package agent

import tea "github.com/charmbracelet/bubbletea"

// Runtime defines the app callbacks needed to process Source 2 input.
type Runtime interface {
	IsInputIdle() bool
	StreamActive() bool
	InjectTaskNotificationContinuation(item Notification) tea.Cmd
}

// Update routes Source 2 (agent -> agent) task-notification messages.
func Update(rt Runtime, state *State, msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(TickMsg); !ok {
		return nil, false
	}
	return handleTaskNotificationTick(rt, state), true
}

func handleTaskNotificationTick(rt Runtime, state *State) tea.Cmd {
	cmds := []tea.Cmd{StartTicker()}

	ResetTrackerIfIdle(rt.StreamActive())

	items := PopReadyNotifications(state.Notifications, rt.IsInputIdle())
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, rt.InjectTaskNotificationContinuation(MergeNotifications(items)))
	return tea.Batch(cmds...)
}

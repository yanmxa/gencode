package notify

import tea "github.com/charmbracelet/bubbletea"

// Deps holds the app-level state and callbacks needed to process incoming messages.
type Deps struct {
	StreamActive bool
	Inject       func(Message) tea.Cmd
}

// Update routes incoming messages from other agents.
func Update(deps Deps, state *Model, msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(TickMsg); !ok {
		return nil, false
	}
	return handleTick(deps, state), true
}

func handleTick(deps Deps, state *Model) tea.Cmd {
	cmds := []tea.Cmd{StartTicker()}

	if state.BGTracker != nil {
		state.BGTracker.ResetIfIdle(deps.StreamActive)
	}

	items := PopReady(state.Queue, !deps.StreamActive)
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, deps.Inject(Merge(items)))
	return tea.Batch(cmds...)
}

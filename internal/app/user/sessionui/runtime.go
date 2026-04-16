package sessionui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Runtime defines the callbacks the sessionui package needs from the parent app model.
type Runtime interface {
	EnsureSessionStore() error
	ForkSession(id string) (forkedID string, err error)
	LoadSession(id string) error
	AddNotice(text string)
	ResetCommitIndex()
	CommitAllMessages() []tea.Cmd
}

// Update routes session selection messages.
func Update(rt Runtime, state *State, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case SelectedMsg:
		return handleSessionSelected(rt, state, msg), true
	}
	return nil, false
}

func handleSessionSelected(rt Runtime, state *State, msg SelectedMsg) tea.Cmd {
	sessionID := msg.SessionID

	// If fork is pending, fork the selected session instead of resuming it directly.
	if state.PendingFork {
		state.PendingFork = false
		if err := rt.EnsureSessionStore(); err != nil {
			rt.AddNotice("Failed to fork session: " + err.Error())
			return nil
		}
		forkedID, err := rt.ForkSession(sessionID)
		if err != nil {
			rt.AddNotice("Failed to fork session: " + err.Error())
			return nil
		}
		sessionID = forkedID
	}

	if err := rt.LoadSession(sessionID); err != nil {
		rt.AddNotice("Failed to load session: " + err.Error())
	}

	// Commit restored messages to scrollback
	rt.ResetCommitIndex()
	return tea.Batch(rt.CommitAllMessages()...)
}

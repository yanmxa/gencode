package providerui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// State holds provider UI state for the TUI model.
// Domain state (LLM, Store, CurrentModel, tokens, thinking) lives
// on the parent app model, not here.
type State struct {
	FetchingLimits bool
	Selector       Model
	StatusMessage  string // Temporary status shown in status bar
}

// StatusExpiredMsg signals that the temporary status message should be cleared.
type StatusExpiredMsg struct{}

// StatusTimer returns a tea.Cmd that clears the status message after the given duration.
func StatusTimer(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return StatusExpiredMsg{}
	})
}

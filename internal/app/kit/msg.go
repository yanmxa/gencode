package kit

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DismissedMsg is sent when any selector or overlay is dismissed without selection.
type DismissedMsg struct{}

// StatusExpiredMsg signals that a temporary status message should be cleared.
type StatusExpiredMsg struct{}

// StatusTimer returns a tea.Cmd that clears the status message after the given duration.
func StatusTimer(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return StatusExpiredMsg{}
	})
}

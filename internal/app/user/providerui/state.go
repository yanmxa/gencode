package providerui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
)

// State holds provider UI state for the TUI model.
// Domain state (LLM, Store, CurrentModel, tokens, thinking) lives
// on the parent app model, not here.
type State struct {
	FetchingLimits bool
	Selector       Model
	StatusMessage  string // Temporary status shown in status bar
}

// SetStatusMessage sets the temporary status message displayed in the status bar.
func (s *State) SetStatusMessage(msg string) {
	s.StatusMessage = msg
}

// StatusExpiredMsg is an alias for kit.StatusExpiredMsg for backward compatibility.
type StatusExpiredMsg = kit.StatusExpiredMsg

// StatusTimer delegates to kit.StatusTimer.
func StatusTimer(d time.Duration) tea.Cmd {
	return kit.StatusTimer(d)
}

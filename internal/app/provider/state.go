package provider

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/provider"
)

// State holds all provider-related state for the TUI model.
type State struct {
	LLM            provider.LLMProvider
	Store          *provider.Store
	CurrentModel   *provider.CurrentModelInfo
	InputTokens    int
	OutputTokens   int
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

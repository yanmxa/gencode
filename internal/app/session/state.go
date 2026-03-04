package session

import "github.com/yanmxa/gencode/internal/session"

// State holds all session-related state for the TUI model.
type State struct {
	Store           *session.Store
	CurrentID       string
	Selector        Model
	PendingSelector bool
	Memory          string // Loaded session memory (from compaction summary)
}

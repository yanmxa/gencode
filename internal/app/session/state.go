package session

// State holds all session-related state for the TUI model.
type State struct {
	Store           *Store
	CurrentID       string
	Selector        Model
	PendingSelector bool
	PendingFork     bool   // Fork after session selection
	Summary         string // Loaded session summary (from compaction)
}

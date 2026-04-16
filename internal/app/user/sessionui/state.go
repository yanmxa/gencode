package sessionui

// State holds session selector UI state for the TUI model.
type State struct {
	Selector        Model
	PendingSelector bool
	PendingFork     bool // Fork after session selection
}

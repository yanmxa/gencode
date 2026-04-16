package sessionui

// State holds session selector UI state for the TUI model.
// Session domain state (Store, CurrentID, Summary) lives on the parent app model.
type State struct {
	Selector        Model
	PendingSelector bool
}

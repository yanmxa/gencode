package memory

// State holds memory selector UI state for the TUI model.
// Cached instructions (User, Project) live on the parent app model, not here.
type State struct {
	Selector    Model
	EditingFile string
}

// EditorFinishedMsg is sent when the external memory editor closes.
type EditorFinishedMsg struct {
	Err error
}

package memory

// State holds all memory-related state for the TUI model.
type State struct {
	Selector    Model
	EditingFile string
}

// EditorFinishedMsg is sent when the external memory editor closes.
type EditorFinishedMsg struct {
	Err error
}

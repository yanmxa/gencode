package memory

// State holds all memory-related state for the TUI model.
type State struct {
	Selector      Model
	EditingFile   string
	CachedUser    string // Cached user-level instructions (~/.gen/GEN.md + rules)
	CachedProject string // Cached project-level instructions (.gen/GEN.md + rules + local)
}

// EditorFinishedMsg is sent when the external memory editor closes.
type EditorFinishedMsg struct {
	Err error
}

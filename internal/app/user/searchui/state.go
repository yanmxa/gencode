package searchui

// State holds search provider selector state for the TUI model.
// Model is embedded so callers access IsActive/HandleKeypress/Render directly.
type State struct {
	Model
}

package compact

// State holds all compact-related state for the TUI model.
type State struct {
	Active       bool
	Focus        string
	AutoContinue bool
}

// Reset clears all compact state.
func (c *State) Reset() {
	c.Active = false
	c.Focus = ""
	c.AutoContinue = false
}

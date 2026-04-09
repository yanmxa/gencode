package compact

const PhaseSummarizing = "Summarizing conversation history"

// State holds all compact-related state for the TUI model.
type State struct {
	Active          bool
	Focus           string
	AutoContinue    bool
	LastResult      string
	LastError       bool
	Phase           string
	WarningSuppressed bool
}

// Reset clears all compact state.
func (c *State) Reset() {
	c.Active = false
	c.Focus = ""
	c.AutoContinue = false
	c.LastResult = ""
	c.LastError = false
	c.Phase = ""
	c.WarningSuppressed = false
}

// ClearResult dismisses the last visible compact status.
func (c *State) ClearResult() {
	c.LastResult = ""
	c.LastError = false
}

// Complete transitions compact state from running to a visible result state.
func (c *State) Complete(result string, isError bool) {
	c.Active = false
	c.Focus = ""
	c.AutoContinue = false
	c.LastResult = result
	c.LastError = isError
	c.Phase = ""
	if !isError {
		c.WarningSuppressed = true
	}
}

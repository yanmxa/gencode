package conversation

// StreamState holds streaming-related display state for the TUI.
// The core.Agent owns the actual stream channels and cancellation;
// the TUI only tracks whether streaming is active for display purposes.
type StreamState struct {
	Active       bool
	BuildingTool string
}

// Stop clears streaming state.
func (s *StreamState) Stop() {
	s.Active = false
	s.BuildingTool = ""
}

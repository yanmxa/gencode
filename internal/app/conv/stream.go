package conv

// StreamState holds streaming-related display state for the TUI.
type StreamState struct {
	Active       bool
	BuildingTool string
}

// Stop clears streaming state.
func (s *StreamState) Stop() {
	s.Active = false
	s.BuildingTool = ""
}

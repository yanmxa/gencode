package system

// View returns any system-event indicators to display.
// Currently, cron and async hook status are rendered inline by the status bar;
// this placeholder exists for the architecture target layout.
func (s *State) View() string {
	return ""
}

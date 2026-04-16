package system

// View returns any system-event indicators to display.
// Currently, cron and async hook status are rendered inline by the status bar;
// this placeholder exists for the architecture target layout.
func (s *State) View() string {
	return ""
}

// RenderHookStatus returns the hook status string if set, otherwise the default model name.
func RenderHookStatus(hookStatus, defaultModelName string) string {
	if hookStatus != "" {
		return hookStatus
	}
	return defaultModelName
}

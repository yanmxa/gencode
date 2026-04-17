// Package options defines configuration types shared across app and tui packages.
package setting

// RunOptions contains all options for running the application.
type RunOptions struct {
	Print     string // non-empty → non-interactive print mode
	Prompt    string // initial prompt for interactive TUI
	PluginDir string
	PlanMode  bool   // enter plan mode
	Continue  bool   // resume most recent session
	Resume    bool   // open session selector or resume by ID
	ResumeID  string // specific session ID to resume
}

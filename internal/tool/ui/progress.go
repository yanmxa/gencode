package ui

import "fmt"

// SpinnerFrames contains the spinner animation frames
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// RenderProgress renders an in-progress state
// ⠋ Reading file...
// ⠙ Searching 42 files...
// ⠹ Fetching https://example.com...
func RenderProgress(spinnerFrame string, message string) string {
	return fmt.Sprintf("%s %s",
		SpinnerStyle.Render(spinnerFrame),
		ProgressMsgStyle.Render(message))
}

// GetProgressMessage returns the appropriate progress message for a tool
func GetProgressMessage(toolName string, args string) string {
	switch toolName {
	case "Read":
		return fmt.Sprintf("Reading %s...", args)
	case "Glob":
		return fmt.Sprintf("Searching for %s...", args)
	case "Grep":
		return fmt.Sprintf("Searching pattern %s...", args)
	case "WebFetch":
		return fmt.Sprintf("Fetching %s...", args)
	default:
		return fmt.Sprintf("Executing %s...", toolName)
	}
}

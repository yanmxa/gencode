// Package agentui provides the agent-type selector overlay UI.
// This is distinct from internal/app/agent/ which handles Source 2
// (agent → agent) inputs: background task notifications and batch tracking.
package agentui

// State holds all agent-related state for the TUI model.
// Model is embedded so callers access IsActive/HandleKeypress/Render directly.
type State struct {
	Model
}

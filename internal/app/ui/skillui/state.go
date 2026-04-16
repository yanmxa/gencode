package skillui

// State holds all skill-related state for the TUI model.
type State struct {
	Selector            Model
	PendingInstructions string
	PendingArgs         string
	ActiveInvocation    string // Persisted skill instructions injected into system prompt across turns
}

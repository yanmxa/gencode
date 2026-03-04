package skill

// State holds all skill-related state for the TUI model.
type State struct {
	Selector            Model
	PendingInstructions string
	PendingArgs         string
}

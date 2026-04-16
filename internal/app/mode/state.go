package mode

// State holds plan mode modal prompt UI components.
// Domain state (PlanEnabled, PlanTask, PlanStore, OperationMode, etc.) lives on
// the parent app model, not here.
type State struct {
	PlanApproval *PlanPrompt
	PlanEntry    *EnterPlanPrompt
	Question     *QuestionPrompt
}

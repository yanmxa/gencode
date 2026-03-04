package mode

import (
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// OperationMode represents the current operation mode of the application.
type OperationMode int

const (
	Normal     OperationMode = iota
	AutoAccept               // auto-accept file edits
	Plan                     // plan-only mode (no edits)
)

// Next cycles to the next operation mode.
func (m OperationMode) Next() OperationMode {
	return (m + 1) % 3
}

// State holds all operation mode and plan state for the TUI model.
type State struct {
	// Operation mode
	Operation          OperationMode
	SessionPermissions *config.SessionPermissions
	DisabledTools      map[string]bool

	// Plan
	Enabled         bool
	Task            string
	Store           *plan.Store
	PlanApproval    *PlanPrompt
	PlanEntry       *EnterPlanPrompt
	Question        *QuestionPrompt
	PendingQuestion *tool.QuestionRequest
}

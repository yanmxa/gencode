package mode

import (
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// OperationMode represents the current operation mode of the application.
type OperationMode int

const (
	Normal            OperationMode = iota
	AutoAccept                      // auto-accept file edits
	Plan                            // plan-only mode (no edits)
	BypassPermissions               // bypass all permissions (entered via hooks, not cycling)
)

var cycleModes = []OperationMode{Normal, AutoAccept, Plan}
var cycleModesWithBypass = []OperationMode{Normal, AutoAccept, Plan, BypassPermissions}

// Next cycles to the next operation mode.
func (m OperationMode) Next() OperationMode {
	for i, mode := range cycleModes {
		if mode == m {
			return cycleModes[(i+1)%len(cycleModes)]
		}
	}
	return Normal
}

// NextWithBypass cycles to the next operation mode.
// When enabled is true, BypassPermissions is included in the cycle.
func (m OperationMode) NextWithBypass(enabled bool) OperationMode {
	modes := cycleModes
	if enabled {
		modes = cycleModesWithBypass
	}
	for i, mode := range modes {
		if mode == m {
			return modes[(i+1)%len(modes)]
		}
	}
	return Normal
}

// State holds all operation mode and plan state for the TUI model.
type State struct {
	// Operation mode
	Operation          OperationMode
	SessionPermissions *config.SessionPermissions
	DisabledTools      map[string]bool

	// Plan
	Enabled              bool
	Task                 string
	Store                *plan.Store
	PlanApproval         *PlanPrompt
	PlanEntry            *EnterPlanPrompt
	Question             *QuestionPrompt
	PendingQuestion      *tool.QuestionRequest
	PendingQuestionReply chan *tool.QuestionResponse
}

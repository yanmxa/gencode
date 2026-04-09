package mode

import (
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// State holds all operation mode and plan state for the TUI model.
type State struct {
	// Operation mode
	Operation          config.OperationMode
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

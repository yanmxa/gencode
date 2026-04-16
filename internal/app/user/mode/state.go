package mode

import (
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// State holds plan mode UI state and modal prompts.
// Domain state (OperationMode, SessionPermissions, DisabledTools) lives on the
// parent app model, not here.
type State struct {
	Enabled              bool
	Task                 string
	Store                *plan.Store
	PlanApproval         *PlanPrompt
	PlanEntry            *EnterPlanPrompt
	Question             *QuestionPrompt
	PendingQuestion      *tool.QuestionRequest
	PendingQuestionReply chan *tool.QuestionResponse
}

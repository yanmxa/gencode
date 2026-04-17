package modal

import (
	"github.com/yanmxa/gencode/internal/tool"
)

// PlanRequestMsg is sent when ExitPlanMode tool is called
type PlanRequestMsg struct {
	Request *tool.PlanRequest
}

// PlanResponseMsg is sent when user responds to plan approval
type PlanResponseMsg struct {
	Request      *tool.PlanRequest
	Response     *tool.PlanResponse
	Approved     bool
	ApproveMode  string // "clear-auto" | "auto" | "manual" | "modify"
	ModifiedPlan string // Modified plan if edited
}

// EnterPlanRequestMsg is sent when EnterPlanMode tool is called
type EnterPlanRequestMsg struct {
	Request *tool.EnterPlanRequest
}

// EnterPlanResponseMsg is sent when user responds
type EnterPlanResponseMsg struct {
	Request  *tool.EnterPlanRequest
	Response *tool.EnterPlanResponse
	Approved bool
}

// QuestionRequestMsg is sent when AskUserQuestion tool is called
type QuestionRequestMsg struct {
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// QuestionResponseMsg is sent when user answers or cancels
type QuestionResponseMsg struct {
	Request   *tool.QuestionRequest
	Response  *tool.QuestionResponse
	Cancelled bool
}

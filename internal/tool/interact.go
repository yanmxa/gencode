package tool

import "context"

// --- AskUser types ---

// QuestionOption represents a single option for a question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Question represents a question to ask the user.
type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

// QuestionRequest is sent to the TUI to display questions.
type QuestionRequest struct {
	ID        string
	Questions []Question
}

// QuestionResponse contains the user's answers.
type QuestionResponse struct {
	RequestID string
	Answers   map[int][]string
	Cancelled bool
}

// AskQuestionFunc requests a question response from an interactive caller.
type AskQuestionFunc func(ctx context.Context, req *QuestionRequest) (*QuestionResponse, error)

// --- EnterPlanMode types ---

// EnterPlanRequest is sent to the TUI to ask user consent to enter plan mode.
type EnterPlanRequest struct {
	ID      string
	Message string
}

// EnterPlanResponse contains the user's decision.
type EnterPlanResponse struct {
	RequestID string
	Approved  bool
}

// --- ExitPlanMode types ---

// PlanRequest is sent to the TUI to display plan for approval.
type PlanRequest struct {
	ID   string
	Plan string
}

// PlanResponse contains the user's decision.
type PlanResponse struct {
	RequestID    string
	Approved     bool
	ApproveMode  string
	ModifiedPlan string
}

// --- Worktree types ---

// EnterWorktreeRequest is sent to the TUI for user confirmation.
type EnterWorktreeRequest struct {
	Slug string
}

// EnterWorktreeResponse is the TUI's response.
type EnterWorktreeResponse struct {
	Approved      bool
	WorktreePath  string
	WorktreeClean func()
}

// ExitWorktreeRequest is sent to the TUI for user confirmation.
type ExitWorktreeRequest struct {
	Action         string
	DiscardChanges bool
}

// ExitWorktreeResponse is the TUI's response.
type ExitWorktreeResponse struct {
	Approved     bool
	RestoredPath string
}

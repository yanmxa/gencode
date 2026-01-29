package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// PlanRequest is sent to the TUI to display plan for approval
type PlanRequest struct {
	ID   string // Unique identifier for this request
	Plan string // The implementation plan content (markdown)
}

// PlanResponse contains the user's decision
type PlanResponse struct {
	RequestID    string // ID of the original request
	Approved     bool   // Whether user approved the plan
	ApproveMode  string // "clear-auto" | "auto" | "manual" | "modify"
	ModifiedPlan string // Modified plan content (if ApproveMode is "modify")
}

// ExitPlanModeTool signals completion of plan mode
type ExitPlanModeTool struct {
	requestCounter int
}

// NewExitPlanModeTool creates a new ExitPlanModeTool
func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{}
}

func (t *ExitPlanModeTool) Name() string {
	return "ExitPlanMode"
}

func (t *ExitPlanModeTool) Description() string {
	return "Exit plan mode and submit the implementation plan for user approval. Call this when you have finished exploring and created a complete plan."
}

func (t *ExitPlanModeTool) Icon() string {
	return "📋"
}

func (t *ExitPlanModeTool) RequiresInteraction() bool {
	return true
}

// PrepareInteraction parses parameters and returns a PlanRequest
func (t *ExitPlanModeTool) PrepareInteraction(ctx context.Context, params map[string]any, cwd string) (any, error) {
	planContent, ok := params["plan"].(string)
	if !ok || planContent == "" {
		return nil, fmt.Errorf("missing required parameter: plan (the implementation plan content)")
	}

	t.requestCounter++
	return &PlanRequest{
		ID:   fmt.Sprintf("plan-%d", t.requestCounter),
		Plan: planContent,
	}, nil
}

// ExecuteWithResponse handles the user's approval decision
func (t *ExitPlanModeTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) ui.ToolResult {
	resp, ok := response.(*PlanResponse)
	if !ok {
		return ui.NewErrorResult("ExitPlanMode", "invalid response type")
	}

	if !resp.Approved {
		return ui.ToolResult{
			Success: true,
			Output:  "Plan was rejected by the user. Please modify the plan based on their feedback and try again.",
			Metadata: ui.ResultMetadata{
				Title:    "ExitPlanMode",
				Icon:     "📋",
				Subtitle: "Rejected",
			},
		}
	}

	// Format response based on approval mode
	modeDesc := map[string]string{
		"clear-auto": "Plan approved. Context cleared. Auto-accept mode enabled for edits.",
		"auto":       "Plan approved. Auto-accept mode enabled for edits.",
		"manual":     "Plan approved. Manual approval mode - each change requires confirmation.",
		"modify":     "Plan modified and approved.",
	}

	description, exists := modeDesc[resp.ApproveMode]
	if !exists {
		description = "Plan approved."
	}

	output := description + "\n\nYou may now proceed with the implementation."

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    "ExitPlanMode",
			Icon:     "📋",
			Subtitle: "Approved",
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *ExitPlanModeTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return ui.NewErrorResult("ExitPlanMode", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	Register(NewExitPlanModeTool())
}

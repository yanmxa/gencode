package mode

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

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
	planContent := tool.GetString(params, "plan")
	if planContent == "" {
		return nil, fmt.Errorf("missing required parameter: plan (the implementation plan content)")
	}

	t.requestCounter++
	return &tool.PlanRequest{
		ID:   fmt.Sprintf("plan-%d", t.requestCounter),
		Plan: planContent,
	}, nil
}

// ExecuteWithResponse handles the user's approval decision
func (t *ExitPlanModeTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) toolresult.ToolResult {
	resp, ok := response.(*tool.PlanResponse)
	if !ok {
		return toolresult.NewErrorResult("ExitPlanMode", "invalid response type")
	}

	if !resp.Approved {
		return toolresult.ToolResult{
			Success: true,
			Output:  "Plan was rejected by the user. Please modify the plan based on their feedback and try again.",
			Metadata: toolresult.ResultMetadata{
				Title:    "ExitPlanMode",
				Icon:     "📋",
				Subtitle: "Rejected",
			},
		}
	}

	// Handle "modify" — user gave feedback, stay in plan mode
	if resp.ApproveMode == "modify" {
		output := "The user wants changes to the plan. You are still in plan mode. Please revise your plan based on the user's feedback below, then call ExitPlanMode again with the updated plan.\n\n"
		if resp.ModifiedPlan != "" {
			output += resp.ModifiedPlan
		}
		return toolresult.ToolResult{
			Success: true,
			Output:  output,
			Metadata: toolresult.ResultMetadata{
				Title:    "ExitPlanMode",
				Icon:     "📋",
				Subtitle: "Revision requested",
			},
		}
	}

	// Format response based on approval mode
	modeDesc := map[string]string{
		"clear-auto": "Plan approved. Context cleared. Auto-accept mode enabled for edits.",
		"auto":       "Plan approved. Auto-accept mode enabled for edits.",
		"manual":     "Plan approved. Manual approval mode - each change requires confirmation.",
	}

	description, exists := modeDesc[resp.ApproveMode]
	if !exists {
		description = "Plan approved."
	}

	output := description + "\n\nStart implementing the plan now. Do NOT explore or investigate further — proceed directly to making code changes step by step."

	return toolresult.ToolResult{
		Success: true,
		Output:  output,
		Metadata: toolresult.ResultMetadata{
			Title:    "ExitPlanMode",
			Icon:     "📋",
			Subtitle: "Approved",
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *ExitPlanModeTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return toolresult.NewErrorResult("ExitPlanMode", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	tool.Register(NewExitPlanModeTool())
}

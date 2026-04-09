package mode

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// EnterPlanModeTool allows AI to request entering plan mode
type EnterPlanModeTool struct {
	requestCounter int
}

// NewEnterPlanModeTool creates a new EnterPlanModeTool
func NewEnterPlanModeTool() *EnterPlanModeTool {
	return &EnterPlanModeTool{}
}

func (t *EnterPlanModeTool) Name() string {
	return "EnterPlanMode"
}

func (t *EnterPlanModeTool) Description() string {
	return "Request to enter plan mode for complex implementation tasks. Use this when a task requires exploration and planning before making changes. The user must approve entering plan mode."
}

func (t *EnterPlanModeTool) Icon() string {
	return "📋"
}

func (t *EnterPlanModeTool) RequiresInteraction() bool {
	return true
}

// PrepareInteraction parses parameters and returns an EnterPlanRequest
func (t *EnterPlanModeTool) PrepareInteraction(ctx context.Context, params map[string]any, cwd string) (any, error) {
	message := tool.GetString(params, "message")

	t.requestCounter++
	return &tool.EnterPlanRequest{
		ID:      fmt.Sprintf("enter-plan-%d", t.requestCounter),
		Message: message,
	}, nil
}

// ExecuteWithResponse handles the user's approval decision
func (t *EnterPlanModeTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) toolresult.ToolResult {
	resp, ok := response.(*tool.EnterPlanResponse)
	if !ok {
		return toolresult.NewErrorResult("EnterPlanMode", "invalid response type")
	}

	if !resp.Approved {
		return toolresult.ToolResult{
			Success: true,
			Output:  "User declined to enter plan mode. Proceed with the task using available tools, or ask the user for clarification on how they would like to proceed.",
			Metadata: toolresult.ResultMetadata{
				Title:    "EnterPlanMode",
				Icon:     "📋",
				Subtitle: "Declined",
			},
		}
	}

	return toolresult.ToolResult{
		Success: true,
		Output:  "User approved entering plan mode. You are now in plan mode. You have access to read-only tools (Read, Glob, Grep, WebFetch, WebSearch), the Agent tool for spawning Explore and Plan subagents, and ExitPlanMode to submit your final plan. Follow the plan mode workflow in your instructions.",
		Metadata: toolresult.ResultMetadata{
			Title:    "EnterPlanMode",
			Icon:     "📋",
			Subtitle: "Approved",
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *EnterPlanModeTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return toolresult.NewErrorResult("EnterPlanMode", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	tool.Register(NewEnterPlanModeTool())
}

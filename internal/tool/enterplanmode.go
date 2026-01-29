package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// EnterPlanRequest is sent to the TUI to ask user consent to enter plan mode
type EnterPlanRequest struct {
	ID      string // Unique identifier for this request
	Message string // Optional message explaining why plan mode is needed
}

// EnterPlanResponse contains the user's decision
type EnterPlanResponse struct {
	RequestID string // ID of the original request
	Approved  bool   // Whether user approved entering plan mode
}

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
	// Optional message parameter
	message := ""
	if msg, ok := params["message"].(string); ok {
		message = msg
	}

	t.requestCounter++
	return &EnterPlanRequest{
		ID:      fmt.Sprintf("enter-plan-%d", t.requestCounter),
		Message: message,
	}, nil
}

// ExecuteWithResponse handles the user's approval decision
func (t *EnterPlanModeTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) ui.ToolResult {
	resp, ok := response.(*EnterPlanResponse)
	if !ok {
		return ui.NewErrorResult("EnterPlanMode", "invalid response type")
	}

	if !resp.Approved {
		return ui.ToolResult{
			Success: true,
			Output:  "User declined to enter plan mode. Proceed with the task using available tools, or ask the user for clarification on how they would like to proceed.",
			Metadata: ui.ResultMetadata{
				Title:    "EnterPlanMode",
				Icon:     "📋",
				Subtitle: "Declined",
			},
		}
	}

	return ui.ToolResult{
		Success: true,
		Output:  "User approved entering plan mode. You are now in plan mode. Explore the codebase using read-only tools (Read, Glob, Grep, WebFetch, WebSearch) to understand the context and create an implementation plan. When your plan is ready, use ExitPlanMode to submit it for user approval.",
		Metadata: ui.ResultMetadata{
			Title:    "EnterPlanMode",
			Icon:     "📋",
			Subtitle: "Approved",
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *EnterPlanModeTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return ui.NewErrorResult("EnterPlanMode", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	Register(NewEnterPlanModeTool())
}

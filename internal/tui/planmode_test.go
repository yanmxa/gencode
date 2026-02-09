package tui

import (
	"testing"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

// TestPlanResponse_ModifyStaysInPlanMode verifies that when user gives feedback
// via option 4 (modify), the model stays in plan mode for plan revision.
func TestPlanResponse_ModifyStaysInPlanMode(t *testing.T) {
	m := &model{
		planMode:      true,
		operationMode: modePlan,
		planTask:      "test task",
		planPrompt:     NewPlanPrompt(),
		questionPrompt: NewQuestionPrompt(),
		pendingToolCalls: []provider.ToolCall{
			{ID: "tc-1", Name: "ExitPlanMode"},
		},
		pendingToolIdx:     0,
		sessionPermissions: config.NewSessionPermissions(),
		messages:           []chatMessage{},
	}

	msg := PlanResponseMsg{
		Request:     &tool.PlanRequest{ID: "plan-1", Plan: "## Original Plan\nDo something"},
		Approved:    true,
		ApproveMode: "modify",
		ModifiedPlan: "## Original Plan\nDo something\n\n---\n\n**User Feedback:**\nAdd error handling",
		Response: &tool.PlanResponse{
			RequestID:    "plan-1",
			Approved:     true,
			ApproveMode:  "modify",
			ModifiedPlan: "## Original Plan\nDo something\n\n---\n\n**User Feedback:**\nAdd error handling",
		},
	}

	m.handlePlanResponse(msg)

	// After modify: should still be in plan mode
	if !m.planMode {
		t.Error("planMode should remain true after modify feedback")
	}
	if m.operationMode != modePlan {
		t.Errorf("operationMode should be modePlan, got %d", m.operationMode)
	}
}

// TestPlanResponse_ManualExitsPlanMode verifies that manual approval exits plan mode.
func TestPlanResponse_ManualExitsPlanMode(t *testing.T) {
	m := &model{
		planMode:      true,
		operationMode: modePlan,
		planTask:      "test task",
		planPrompt:     NewPlanPrompt(),
		questionPrompt: NewQuestionPrompt(),
		pendingToolCalls: []provider.ToolCall{
			{ID: "tc-1", Name: "ExitPlanMode"},
		},
		pendingToolIdx:     0,
		sessionPermissions: config.NewSessionPermissions(),
		messages:           []chatMessage{},
	}

	msg := PlanResponseMsg{
		Request:     &tool.PlanRequest{ID: "plan-1", Plan: "## Plan\nSome plan"},
		Approved:    true,
		ApproveMode: "manual",
		Response: &tool.PlanResponse{
			RequestID:   "plan-1",
			Approved:    true,
			ApproveMode: "manual",
		},
	}

	m.handlePlanResponse(msg)

	if m.planMode {
		t.Error("planMode should be false after manual approval")
	}
	if m.operationMode != modeNormal {
		t.Errorf("operationMode should be modeNormal, got %d", m.operationMode)
	}
}

// TestPlanResponse_AutoExitsPlanMode verifies that auto approval exits plan mode.
func TestPlanResponse_AutoExitsPlanMode(t *testing.T) {
	m := &model{
		planMode:      true,
		operationMode: modePlan,
		planTask:      "test task",
		planPrompt:     NewPlanPrompt(),
		questionPrompt: NewQuestionPrompt(),
		pendingToolCalls: []provider.ToolCall{
			{ID: "tc-1", Name: "ExitPlanMode"},
		},
		pendingToolIdx:     0,
		sessionPermissions: config.NewSessionPermissions(),
		messages:           []chatMessage{},
	}

	msg := PlanResponseMsg{
		Request:     &tool.PlanRequest{ID: "plan-1", Plan: "## Plan\nSome plan"},
		Approved:    true,
		ApproveMode: "auto",
		Response: &tool.PlanResponse{
			RequestID:   "plan-1",
			Approved:    true,
			ApproveMode: "auto",
		},
	}

	m.handlePlanResponse(msg)

	if m.planMode {
		t.Error("planMode should be false after auto approval")
	}
	if m.operationMode != modeAutoAccept {
		t.Errorf("operationMode should be modeAutoAccept, got %d", m.operationMode)
	}
	if !m.sessionPermissions.AllowAllEdits {
		t.Error("auto mode should enable AllowAllEdits")
	}
}

// TestPlanResponse_RejectedExitsPlanMode verifies that rejection exits plan mode.
func TestPlanResponse_RejectedExitsPlanMode(t *testing.T) {
	m := &model{
		planMode:      true,
		operationMode: modePlan,
		planTask:      "test task",
		planPrompt:     NewPlanPrompt(),
		questionPrompt: NewQuestionPrompt(),
		pendingToolCalls: []provider.ToolCall{
			{ID: "tc-1", Name: "ExitPlanMode"},
		},
		pendingToolIdx:     0,
		sessionPermissions: config.NewSessionPermissions(),
		messages:           []chatMessage{},
	}

	msg := PlanResponseMsg{
		Request:  &tool.PlanRequest{ID: "plan-1", Plan: "## Plan\nSome plan"},
		Approved: false,
		Response: &tool.PlanResponse{
			RequestID: "plan-1",
			Approved:  false,
		},
	}

	m.handlePlanResponse(msg)

	if m.planMode {
		t.Error("planMode should be false after rejection")
	}
	if m.operationMode != modeNormal {
		t.Errorf("operationMode should be modeNormal after rejection, got %d", m.operationMode)
	}
	// Should have added a rejection tool result message
	found := false
	for _, msg := range m.messages {
		if msg.toolResult != nil && msg.toolResult.IsError {
			found = true
			break
		}
	}
	if !found {
		t.Error("rejection should add a tool result message with IsError=true")
	}
}

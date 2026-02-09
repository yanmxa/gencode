package tool

import (
	"context"
	"strings"
	"testing"
)

func TestExitPlanMode_ModifyKeepsPlanMode(t *testing.T) {
	tool := NewExitPlanModeTool()

	// Simulate user giving feedback via option 4
	resp := &PlanResponse{
		RequestID:    "plan-1",
		Approved:     true,
		ApproveMode:  "modify",
		ModifiedPlan: "## Summary\nOriginal plan\n\n---\n\n**User Feedback:**\nPlease add error handling",
	}

	result := tool.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Output)
	}

	// Should tell LLM to revise, NOT to proceed with implementation
	if strings.Contains(result.Output, "proceed with the implementation") {
		t.Error("modify response should NOT tell LLM to proceed with implementation")
	}
	if !strings.Contains(result.Output, "revise") {
		t.Error("modify response should tell LLM to revise the plan")
	}
	if !strings.Contains(result.Output, "still in plan mode") {
		t.Error("modify response should tell LLM it's still in plan mode")
	}
	if !strings.Contains(result.Output, "ExitPlanMode") {
		t.Error("modify response should tell LLM to call ExitPlanMode again")
	}
	// Should include the user's feedback
	if !strings.Contains(result.Output, "Please add error handling") {
		t.Error("modify response should include user feedback")
	}
	// Subtitle should indicate revision
	if result.Metadata.Subtitle != "Revision requested" {
		t.Errorf("expected subtitle 'Revision requested', got %q", result.Metadata.Subtitle)
	}
}

func TestExitPlanMode_ApprovalModes(t *testing.T) {
	tool := NewExitPlanModeTool()

	tests := []struct {
		mode       string
		wantSubstr string
	}{
		{"clear-auto", "Context cleared"},
		{"auto", "Auto-accept mode"},
		{"manual", "Manual approval"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			resp := &PlanResponse{
				RequestID:   "plan-1",
				Approved:    true,
				ApproveMode: tt.mode,
			}
			result := tool.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")
			if !result.Success {
				t.Errorf("expected success for mode %s", tt.mode)
			}
			if !strings.Contains(result.Output, tt.wantSubstr) {
				t.Errorf("mode %s: expected %q in output, got %q", tt.mode, tt.wantSubstr, result.Output)
			}
			if !strings.Contains(result.Output, "proceed with the implementation") {
				t.Errorf("mode %s: should tell LLM to proceed", tt.mode)
			}
			if result.Metadata.Subtitle != "Approved" {
				t.Errorf("mode %s: expected subtitle 'Approved', got %q", tt.mode, result.Metadata.Subtitle)
			}
		})
	}
}

func TestExitPlanMode_Rejected(t *testing.T) {
	tool := NewExitPlanModeTool()

	resp := &PlanResponse{
		RequestID: "plan-1",
		Approved:  false,
	}
	result := tool.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")

	if !result.Success {
		t.Error("rejected should still return success (not a tool error)")
	}
	if !strings.Contains(result.Output, "rejected") {
		t.Error("should mention plan was rejected")
	}
	if result.Metadata.Subtitle != "Rejected" {
		t.Errorf("expected subtitle 'Rejected', got %q", result.Metadata.Subtitle)
	}
}

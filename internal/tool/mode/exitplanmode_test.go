package mode

import (
	"context"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/provider"
)

func TestExitPlanMode_ModifyKeepsPlanMode(t *testing.T) {
	ep := NewExitPlanModeTool()

	// Simulate user giving feedback via option 4
	resp := &tool.PlanResponse{
		RequestID:    "plan-1",
		Approved:     true,
		ApproveMode:  "modify",
		ModifiedPlan: "## Summary\nOriginal plan\n\n---\n\n**User Feedback:**\nPlease add error handling",
	}

	result := ep.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")

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
	ep := NewExitPlanModeTool()

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
			resp := &tool.PlanResponse{
				RequestID:   "plan-1",
				Approved:    true,
				ApproveMode: tt.mode,
			}
			result := ep.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")
			if !result.Success {
				t.Errorf("expected success for mode %s", tt.mode)
			}
			if !strings.Contains(result.Output, tt.wantSubstr) {
				t.Errorf("mode %s: expected %q in output, got %q", tt.mode, tt.wantSubstr, result.Output)
			}
			if !strings.Contains(result.Output, "Start implementing the plan now") {
				t.Errorf("mode %s: should tell LLM to proceed", tt.mode)
			}
			if result.Metadata.Subtitle != "Approved" {
				t.Errorf("mode %s: expected subtitle 'Approved', got %q", tt.mode, result.Metadata.Subtitle)
			}
		})
	}
}

func TestExitPlanMode_Rejected(t *testing.T) {
	ep := NewExitPlanModeTool()

	resp := &tool.PlanResponse{
		RequestID: "plan-1",
		Approved:  false,
	}
	result := ep.ExecuteWithResponse(context.Background(), nil, resp, "/tmp")

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

func TestPlanMode_BlocksWriteTools(t *testing.T) {
	// In plan mode, Write and Edit are NOT in the tool schema.
	set := &tool.Set{PlanMode: true}
	tools := set.Tools()

	writeBlocked := []string{"Write", "Edit", "Bash", tool.ToolSkill, tool.ToolEnterPlanMode}
	for _, name := range writeBlocked {
		for _, t := range tools {
			if t.Name == name {
				te := t // avoid name shadowing
				_ = te
				// found — this is the failure
				goto found
			}
		}
		continue
	found:
		t.Errorf("plan mode should not expose %q tool, but it was found in tool set", name)
	}
}

func TestPlanMode_AllowsReadTools(t *testing.T) {
	// In plan mode, read/search/question tools and ExitPlanMode must be available.
	set := &tool.Set{PlanMode: true}
	tools := set.Tools()

	toolIndex := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolIndex[t.Name] = true
	}

	required := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch", "AskUserQuestion", tool.ToolExitPlanMode, tool.ToolAgent}
	for _, name := range required {
		if !toolIndex[name] {
			t.Errorf("plan mode should expose %q tool, but it was not found in tool set", name)
		}
	}
}

func TestPlanMode_AgentSchema_IsForegroundAndRestricted(t *testing.T) {
	set := &tool.Set{PlanMode: true}
	tools := set.Tools()

	var agent provider.ToolSchema
	found := false
	for _, t := range tools {
		if t.Name == tool.ToolAgent {
			agent = t
			found = true
			break
		}
	}
	if !found {
		t.Fatal("plan mode Agent tool not found")
	}

	params, ok := agent.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("Agent parameters missing schema map: %#v", agent.Parameters)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Agent parameters missing properties map: %#v", agent.Parameters)
	}
	if _, exists := props["run_in_background"]; exists {
		t.Error("plan mode Agent should not expose run_in_background")
	}

	subagentType, ok := props["subagent_type"].(map[string]any)
	if !ok {
		t.Fatalf("subagent_type schema missing: %#v", props["subagent_type"])
	}
	desc, _ := subagentType["description"].(string)
	if !strings.Contains(desc, "Explore or Plan") {
		t.Errorf("unexpected subagent_type description: %q", desc)
	}

	if strings.Contains(agent.Description, "Bash") {
		t.Fatalf("plan mode Agent description must not advertise Bash: %q", agent.Description)
	}
}

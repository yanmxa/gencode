package tool

import (
	"context"
	"strings"
	"testing"
)

func TestEnterPlanMode_PrepareInteractionGeneratesIncrementingRequests(t *testing.T) {
	tool := NewEnterPlanModeTool()

	firstAny, err := tool.PrepareInteraction(context.Background(), map[string]any{
		"message": "Need to inspect the codebase first.",
	}, "/tmp")
	if err != nil {
		t.Fatalf("PrepareInteraction(first): %v", err)
	}
	first, ok := firstAny.(*EnterPlanRequest)
	if !ok {
		t.Fatalf("unexpected request type: %#v", firstAny)
	}
	if first.ID != "enter-plan-1" {
		t.Fatalf("first request ID = %q, want %q", first.ID, "enter-plan-1")
	}
	if first.Message != "Need to inspect the codebase first." {
		t.Fatalf("first request message = %q", first.Message)
	}

	secondAny, err := tool.PrepareInteraction(context.Background(), nil, "/tmp")
	if err != nil {
		t.Fatalf("PrepareInteraction(second): %v", err)
	}
	second := secondAny.(*EnterPlanRequest)
	if second.ID != "enter-plan-2" {
		t.Fatalf("second request ID = %q, want %q", second.ID, "enter-plan-2")
	}
	if second.Message != "" {
		t.Fatalf("expected empty message by default, got %q", second.Message)
	}
}

func TestEnterPlanMode_ExecuteWithResponseApprovedAndDeclined(t *testing.T) {
	tool := NewEnterPlanModeTool()

	approved := tool.ExecuteWithResponse(context.Background(), nil, &EnterPlanResponse{
		RequestID: "enter-plan-1",
		Approved:  true,
	}, "/tmp")
	if !approved.Success {
		t.Fatalf("approved response should succeed: %s", approved.Error)
	}
	if approved.Metadata.Subtitle != "Approved" {
		t.Fatalf("approved subtitle = %q", approved.Metadata.Subtitle)
	}
	if !strings.Contains(approved.Output, "now in plan mode") {
		t.Fatalf("approved output should describe entering plan mode, got %q", approved.Output)
	}
	if !strings.Contains(approved.Output, "ExitPlanMode") {
		t.Fatalf("approved output should mention ExitPlanMode, got %q", approved.Output)
	}

	declined := tool.ExecuteWithResponse(context.Background(), nil, &EnterPlanResponse{
		RequestID: "enter-plan-1",
		Approved:  false,
	}, "/tmp")
	if !declined.Success {
		t.Fatalf("declined response should still succeed: %s", declined.Error)
	}
	if declined.Metadata.Subtitle != "Declined" {
		t.Fatalf("declined subtitle = %q", declined.Metadata.Subtitle)
	}
	if !strings.Contains(declined.Output, "User declined to enter plan mode") {
		t.Fatalf("declined output mismatch: %q", declined.Output)
	}
}

func TestEnterPlanMode_ExecuteRejectsDirectInvocation(t *testing.T) {
	tool := NewEnterPlanModeTool()
	result := tool.Execute(context.Background(), nil, "/tmp")

	if result.Success {
		t.Fatal("direct Execute should fail for interactive tool")
	}
	if !strings.Contains(result.Error, "requires user interaction") {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

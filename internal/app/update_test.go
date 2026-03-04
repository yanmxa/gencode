package app

import (
	"strings"
	"testing"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
)

// TestPlanResponse_ModifyStaysInPlanMode verifies that when user gives feedback
// via option 4 (modify), the model stays in plan mode for plan revision.
func TestPlanResponse_ModifyStaysInPlanMode(t *testing.T) {
	m := &model{
		mode: appmode.State{
			Operation:          appmode.Plan,
			SessionPermissions: config.NewSessionPermissions(),
			Enabled:            true,
			Task:               "test task",
			PlanApproval:       appmode.NewPlanPrompt(),
			Question:           appmode.NewQuestionPrompt(),
		},
		tool: apptool.State{
			ExecState: apptool.ExecState{
				PendingCalls: []message.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: appconv.New(),
	}

	msg := appmode.PlanResponseMsg{
		Request:      &tool.PlanRequest{ID: "plan-1", Plan: "## Original Plan\nDo something"},
		Approved:     true,
		ApproveMode:  "modify",
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
	if !m.mode.Enabled {
		t.Error("plan.enabled should remain true after modify feedback")
	}
	if m.mode.Operation != appmode.Plan {
		t.Errorf("operationMode should be appmode.Plan, got %d", m.mode.Operation)
	}
}

// TestPlanResponse_ManualExitsPlanMode verifies that manual approval exits plan mode.
func TestPlanResponse_ManualExitsPlanMode(t *testing.T) {
	m := &model{
		mode: appmode.State{
			Operation:          appmode.Plan,
			SessionPermissions: config.NewSessionPermissions(),
			Enabled:            true,
			Task:               "test task",
			PlanApproval:       appmode.NewPlanPrompt(),
			Question:           appmode.NewQuestionPrompt(),
		},
		tool: apptool.State{
			ExecState: apptool.ExecState{
				PendingCalls: []message.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: appconv.New(),
	}

	msg := appmode.PlanResponseMsg{
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

	if m.mode.Enabled {
		t.Error("plan.enabled should be false after manual approval")
	}
	if m.mode.Operation != appmode.Normal {
		t.Errorf("operationMode should be appmode.Normal, got %d", m.mode.Operation)
	}
}

// TestPlanResponse_AutoExitsPlanMode verifies that auto approval exits plan mode.
func TestPlanResponse_AutoExitsPlanMode(t *testing.T) {
	m := &model{
		mode: appmode.State{
			Operation:          appmode.Plan,
			SessionPermissions: config.NewSessionPermissions(),
			Enabled:            true,
			Task:               "test task",
			PlanApproval:       appmode.NewPlanPrompt(),
			Question:           appmode.NewQuestionPrompt(),
		},
		tool: apptool.State{
			ExecState: apptool.ExecState{
				PendingCalls: []message.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: appconv.New(),
	}

	msg := appmode.PlanResponseMsg{
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

	if m.mode.Enabled {
		t.Error("plan.enabled should be false after auto approval")
	}
	if m.mode.Operation != appmode.AutoAccept {
		t.Errorf("operationMode should be appmode.AutoAccept, got %d", m.mode.Operation)
	}
	if !m.mode.SessionPermissions.AllowAllEdits {
		t.Error("auto mode should enable AllowAllEdits")
	}
}

// TestPlanResponse_RejectedExitsPlanMode verifies that rejection exits plan mode.
func TestPlanResponse_RejectedExitsPlanMode(t *testing.T) {
	m := &model{
		mode: appmode.State{
			Operation:          appmode.Plan,
			SessionPermissions: config.NewSessionPermissions(),
			Enabled:            true,
			Task:               "test task",
			PlanApproval:       appmode.NewPlanPrompt(),
			Question:           appmode.NewQuestionPrompt(),
		},
		tool: apptool.State{
			ExecState: apptool.ExecState{
				PendingCalls: []message.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: appconv.New(),
	}

	msg := appmode.PlanResponseMsg{
		Request:  &tool.PlanRequest{ID: "plan-1", Plan: "## Plan\nSome plan"},
		Approved: false,
		Response: &tool.PlanResponse{
			RequestID: "plan-1",
			Approved:  false,
		},
	}

	m.handlePlanResponse(msg)

	if m.mode.Enabled {
		t.Error("plan.enabled should be false after rejection")
	}
	if m.mode.Operation != appmode.Normal {
		t.Errorf("operationMode should be appmode.Normal after rejection, got %d", m.mode.Operation)
	}
	// Should have added a rejection tool result message
	found := false
	for _, msg := range m.conv.Messages {
		if msg.ToolResult != nil && msg.ToolResult.IsError {
			found = true
			break
		}
	}
	if !found {
		t.Error("rejection should add a tool result message with IsError=true")
	}
}

// TestSessionMemory_BuildExtraContext verifies that session memory is included
// in the extra context returned by buildExtraContext().
func TestSessionMemory_BuildExtraContext(t *testing.T) {
	summary := "User worked on session store refactoring. Key files: store.go, types.go."
	m := &model{
		session: appsession.State{
			Memory: summary,
		},
		conv: appconv.New(),
	}

	extra := m.buildExtraContext()

	// Find the session-memory entry
	var found bool
	for _, e := range extra {
		if strings.Contains(e, "<session-memory>") {
			found = true
			if !strings.Contains(e, summary) {
				t.Errorf("session-memory block should contain the summary, got: %s", e)
			}
			if !strings.Contains(e, "</session-memory>") {
				t.Errorf("session-memory block missing closing tag, got: %s", e)
			}
		}
	}
	if !found {
		t.Error("buildExtraContext() should include <session-memory> when Memory is set")
	}
}

// TestSessionMemory_EmptyNotIncluded verifies that empty session memory
// does not produce a <session-memory> block.
func TestSessionMemory_EmptyNotIncluded(t *testing.T) {
	m := &model{
		session: appsession.State{Memory: ""},
		conv:    appconv.New(),
	}

	extra := m.buildExtraContext()
	for _, e := range extra {
		if strings.Contains(e, "<session-memory>") {
			t.Error("buildExtraContext() should not include <session-memory> when Memory is empty")
		}
	}
}

// TestSessionMemory_InSystemPrompt verifies that session memory from
// buildExtraContext() actually appears in the final system prompt via BuildPrompt.
func TestSessionMemory_InSystemPrompt(t *testing.T) {
	summary := "Refactored the session package. Added overflow storage."
	m := &model{
		session: appsession.State{
			Memory: summary,
		},
		conv: appconv.New(),
	}

	extra := m.buildExtraContext()

	// Build a system prompt using the extra context (same path as configureLoop)
	prompt := system.BuildPrompt(system.Config{
		Provider: "test",
		Model:    "test-model",
		Cwd:      "/tmp",
		Extra:    extra,
	})

	if !strings.Contains(prompt, "<session-memory>") {
		t.Error("system prompt should contain <session-memory> tag")
	}
	if !strings.Contains(prompt, summary) {
		t.Error("system prompt should contain the session memory summary")
	}
	if !strings.Contains(prompt, "</session-memory>") {
		t.Error("system prompt should contain closing </session-memory> tag")
	}
}

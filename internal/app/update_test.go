package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/app/conv"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// TestPlanResponse_ModifyStaysInPlanMode verifies that when user gives feedback
// via option 4 (modify), the model stays in plan mode for plan revision.
func TestPlanResponse_ModifyStaysInPlanMode(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
			PlanEnabled:        true,
			PlanTask:           "test task",
		},
		mode: conv.ModalState{
			PlanApproval: conv.NewPlanPrompt(),
			Question:     conv.NewQuestionPrompt(),
		},
		tool: conv.ToolState{
			ToolExecState: conv.ToolExecState{
				PendingCalls: []core.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: conv.NewConversation(),
	}

	msg := conv.PlanResponseMsg{
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
	if !m.runtime.PlanEnabled {
		t.Error("plan.enabled should remain true after modify feedback")
	}
	if m.runtime.OperationMode != setting.ModePlan {
		t.Errorf("operationMode should be setting.ModePlan, got %d", m.runtime.OperationMode)
	}
}

// TestPlanResponse_ManualExitsPlanMode verifies that manual approval exits plan mode.
func TestPlanResponse_ManualExitsPlanMode(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
			PlanEnabled:        true,
			PlanTask:           "test task",
		},
		mode: conv.ModalState{
			PlanApproval: conv.NewPlanPrompt(),
			Question:     conv.NewQuestionPrompt(),
		},
		tool: conv.ToolState{
			ToolExecState: conv.ToolExecState{
				PendingCalls: []core.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: conv.NewConversation(),
	}

	msg := conv.PlanResponseMsg{
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

	if m.runtime.PlanEnabled {
		t.Error("plan.enabled should be false after manual approval")
	}
	if m.runtime.OperationMode != setting.ModeNormal {
		t.Errorf("operationMode should be setting.ModeNormal, got %d", m.runtime.OperationMode)
	}
}

// TestPlanResponse_AutoExitsPlanMode verifies that auto approval exits plan mode.
func TestPlanResponse_AutoExitsPlanMode(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
			PlanEnabled:        true,
			PlanTask:           "test task",
		},
		mode: conv.ModalState{
			PlanApproval: conv.NewPlanPrompt(),
			Question:     conv.NewQuestionPrompt(),
		},
		tool: conv.ToolState{
			ToolExecState: conv.ToolExecState{
				PendingCalls: []core.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: conv.NewConversation(),
	}

	msg := conv.PlanResponseMsg{
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

	if m.runtime.PlanEnabled {
		t.Error("plan.enabled should be false after auto approval")
	}
	if m.runtime.OperationMode != setting.ModeAutoAccept {
		t.Errorf("operationMode should be setting.ModeAutoAccept, got %d", m.runtime.OperationMode)
	}
	if !m.runtime.SessionPermissions.AllowAllEdits {
		t.Error("auto mode should enable AllowAllEdits")
	}
}

// TestPlanResponse_RejectedExitsPlanMode verifies that rejection exits plan mode.
func TestPlanResponse_RejectedExitsPlanMode(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
			PlanEnabled:        true,
			PlanTask:           "test task",
		},
		mode: conv.ModalState{
			PlanApproval: conv.NewPlanPrompt(),
			Question:     conv.NewQuestionPrompt(),
		},
		tool: conv.ToolState{
			ToolExecState: conv.ToolExecState{
				PendingCalls: []core.ToolCall{
					{ID: "tc-1", Name: "ExitPlanMode"},
				},
				CurrentIdx: 0,
			},
		},
		conv: conv.NewConversation(),
	}

	msg := conv.PlanResponseMsg{
		Request:  &tool.PlanRequest{ID: "plan-1", Plan: "## Plan\nSome plan"},
		Approved: false,
		Response: &tool.PlanResponse{
			RequestID: "plan-1",
			Approved:  false,
		},
	}

	m.handlePlanResponse(msg)

	if m.runtime.PlanEnabled {
		t.Error("plan.enabled should be false after rejection")
	}
	if m.runtime.OperationMode != setting.ModeNormal {
		t.Errorf("operationMode should be setting.ModeNormal after rejection, got %d", m.runtime.OperationMode)
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

func TestHandleQuestionResponse_ForAgentReplyChannel(t *testing.T) {
	reply := make(chan *tool.QuestionResponse, 1)
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
		},
		pendingQuestion:      &tool.QuestionRequest{ID: "ask-1"},
		pendingQuestionReply: reply,
		mode: conv.ModalState{
			Question: conv.NewQuestionPrompt(),
		},
	}

	resp := &tool.QuestionResponse{
		RequestID: "ask-1",
		Answers: map[int][]string{
			0: {"Patch"},
		},
	}
	cmd := m.handleQuestionResponse(conv.QuestionResponseMsg{
		Request:  &tool.QuestionRequest{ID: "ask-1"},
		Response: resp,
	})

	if cmd != nil {
		t.Fatal("expected no follow-up command for agent question response")
	}
	if m.pendingQuestion != nil {
		t.Fatal("expected pending question to be cleared")
	}
	if m.pendingQuestionReply != nil {
		t.Fatal("expected pending question reply channel to be cleared")
	}

	select {
	case got := <-reply:
		if got != resp {
			t.Fatalf("unexpected response pointer: %#v", got)
		}
	default:
		t.Fatal("expected response to be forwarded to reply channel")
	}
}

// TestSessionSummary_InSystemPrompt verifies that session summary
// appears in the final system prompt when set via system.Build.
func TestSessionSummary_InSystemPrompt(t *testing.T) {
	summary := "Refactored the session package. Added overflow storage."

	sys := system.Build(system.Config{
		Cwd:            "/tmp",
		SessionSummary: "<session-summary>\n" + summary + "\n</session-summary>",
	})
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "<session-summary>") {
		t.Error("system prompt should contain <session-summary> tag")
	}
	if !strings.Contains(prompt, summary) {
		t.Error("system prompt should contain the session summary")
	}
	if !strings.Contains(prompt, "</session-summary>") {
		t.Error("system prompt should contain closing </session-summary> tag")
	}
}

// TestSessionSummary_EmptyNotIncluded verifies that empty session summary
// does not produce a <session-summary> block.
func TestSessionSummary_EmptyNotIncluded(t *testing.T) {
	sys := system.Build(system.Config{
		Cwd:            "/tmp",
		SessionSummary: "",
	})
	prompt := sys.Prompt()

	if strings.Contains(prompt, "<session-summary>") {
		t.Error("system prompt should not contain <session-summary> when SessionSummary is empty")
	}
}

func TestIsExitRequest(t *testing.T) {
	if !isExitRequest("exit") {
		t.Fatal("expected lowercase exit to match")
	}
	if !isExitRequest("ExIt") {
		t.Fatal("expected mixed-case exit to match")
	}
	if isExitRequest("exit now") {
		t.Fatal("did not expect non-exact exit command to match")
	}
}

func TestOverlaySelectorsOrder(t *testing.T) {
	m := &model{}
	got := make([]string, 0, len(m.overlaySelectors()))
	for _, selector := range m.overlaySelectors() {
		got = append(got, fmt.Sprintf("%T", selector))
	}

	want := []string{
		"*input.ProviderSelector",
		"*conv.ToolSelector",
		"*input.SkillSelector",
		"*input.AgentSelector",
		"*input.MCPSelector",
		"*input.PluginSelector",
		"*input.SessionSelector",
		"*input.MemorySelector",
		"*input.SearchSelector",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected selector order:\n got: %v\nwant: %v", got, want)
	}
}

func TestStartPromptSuggestionGeneratesCommand(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			LLMProvider: testLLMProvider{},
		},
		conv: conv.ConversationModel{
			Messages: []core.ChatMessage{
				{Role: core.RoleAssistant, Content: "first"},
				{Role: core.RoleAssistant, Content: "second"},
			},
		},
	}

	cmd := m.startPromptSuggestion()
	if cmd == nil {
		t.Fatal("expected prompt suggestion command")
	}
}

func TestBuildPromptSuggestionRequest(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			LLMProvider: testLLMProvider{},
		},
		conv: conv.ConversationModel{
			Messages: []core.ChatMessage{
				{Role: core.RoleUser, Content: "u1"},
				{Role: core.RoleAssistant, Content: "a1"},
				{Role: core.RoleAssistant, Content: "a2"},
			},
		},
	}

	req, ok := m.buildPromptSuggestionRequest()
	if !ok {
		t.Fatal("expected suggestion request")
	}
	if req.Client == nil {
		t.Fatal("expected client in request")
	}
	if req.SystemPrompt != suggestionSystemPrompt {
		t.Fatalf("unexpected system prompt: %q", req.SystemPrompt)
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != core.RoleUser || last.Content != suggestionUserPrompt {
		t.Fatalf("unexpected tail message: %#v", last)
	}
}

func TestExecuteSubmitRequest_AppendsUserMessageAndStartsProviderTurn(t *testing.T) {
	appCwd = t.TempDir(); base := newBaseModel()
	m := &base
	m.agentOutput = conv.New(80, conv.NewProgressHub(16))
	m.conv = conv.ConversationModel{
		Messages: []core.ChatMessage{
			{Role: core.RoleUser, Content: "previous request"},
		},
	}
	m.runtime.LLMProvider = testLLMProvider{}

	cmd := m.executeSubmitRequest(submitRequest{Input: "请修复这个 bug"})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	// Should have appended the user message to the conversation
	found := false
	for _, msg := range m.conv.Messages {
		if msg.Role == core.RoleUser && msg.Content == "请修复这个 bug" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected user message to be appended to conversation, got %#v", m.conv.Messages)
	}
}


func TestBuildCompactRequest(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			SessionSummary: "existing summary",
		},
		conv: conv.ConversationModel{
			Messages: []core.ChatMessage{
				{Role: core.RoleUser, Content: "hello"},
			},
		},
	}

	req := m.buildCompactRequest("focus text", "manual")
	if req.Focus != "focus text" {
		t.Fatalf("unexpected focus: %q", req.Focus)
	}
	if req.Trigger != "manual" {
		t.Fatalf("unexpected trigger: %q", req.Trigger)
	}
	if req.SessionSummary != "existing summary" {
		t.Fatalf("unexpected session summary: %q", req.SessionSummary)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", req.Messages)
	}
}

func TestBuildLoopExtraIncludesSkillInvocation(t *testing.T) {
	m := &model{}
	m.userInput.Skill = input.SkillState{
		ActiveInvocation: "<skill>Use the active skill</skill>",
	}

	extra := m.buildLoopExtra([]string{"base"})
	if len(extra) != 3 {
		t.Fatalf("expected 3 extra entries, got %d: %#v", len(extra), extra)
	}
	if extra[0] != "base" {
		t.Fatalf("unexpected first extra entry: %#v", extra)
	}
	if !strings.Contains(extra[1], "<coordinator-guidance>") {
		t.Fatalf("expected coordinator guidance entry, got %#v", extra[1])
	}
	if extra[2] != "<skill>Use the active skill</skill>" {
		t.Fatalf("unexpected extra ordering: %#v", extra)
	}
}

func TestBuildLoopSystemIncludesSessionSummary(t *testing.T) {
	m := &model{
		cwd: "/tmp/project",
		runtime: appruntime.Model{
			SessionSummary:            "condensed summary",
			CachedUserInstructions:    "user memory",
			CachedProjectInstructions: "project memory",
		},
	}

	sys := m.buildLoopSystem([]string{"extra"}, nil)
	prompt := sys.Prompt()

	// Verify environment layer includes cwd
	if !strings.Contains(prompt, "/tmp/project") {
		t.Fatalf("expected cwd in prompt, got: %s", prompt[:200])
	}
	// Verify session summary layer
	if !strings.Contains(prompt, "<session-summary>") || !strings.Contains(prompt, "condensed summary") {
		t.Fatalf("expected session summary in prompt")
	}
	// Verify extra layers
	if !strings.Contains(prompt, "extra") {
		t.Fatalf("expected 'extra' content in prompt")
	}
	if !strings.Contains(prompt, "<coordinator-guidance>") {
		t.Fatalf("expected coordinator guidance in prompt")
	}
	// Verify instructions layers
	if !strings.Contains(prompt, "user memory") || !strings.Contains(prompt, "project memory") {
		t.Fatalf("expected user and project memory in prompt")
	}
}


func TestBuildCoordinatorGuidanceEncouragesParallelAuditFanout(t *testing.T) {
	guidance := buildCoordinatorGuidance()
	for _, want := range []string{
		"<coordinator-guidance>",
		"broad audit, review, architecture analysis, refactor plan",
		"Default to launching 3-5 background workers",
		"Avoid broad labels like \"deep codebase audit\"",
		"After launching workers, briefly tell the user what you launched and stop.",
		"do not poll, read output files, or check progress",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("coordinator guidance missing %q:\n%s", want, guidance)
		}
	}
}


func TestDetectThinkingKeywords(t *testing.T) {
	t.Run("high thinking keywords", func(t *testing.T) {
		m := &model{}
		m.runtime.DetectThinkingKeywords("Please think carefully before answering")
		if m.runtime.ThinkingOverride != llm.ThinkingHigh {
			t.Fatalf("expected high thinking override, got %v", m.runtime.ThinkingOverride)
		}
	})

	t.Run("ultra keywords win over high", func(t *testing.T) {
		m := &model{}
		m.runtime.DetectThinkingKeywords("Think hard and ultrathink about this")
		if m.runtime.ThinkingOverride != llm.ThinkingUltra {
			t.Fatalf("expected ultra thinking override, got %v", m.runtime.ThinkingOverride)
		}
	})
}

func TestRenderActiveModalPriority(t *testing.T) {
	m := &model{
		runtime: appruntime.Model{
			OperationMode:      setting.ModePlan,
			SessionPermissions: setting.NewSessionPermissions(),
		},
		mode: conv.ModalState{
			PlanApproval: conv.NewPlanPrompt(),
			Question:     conv.NewQuestionPrompt(),
			PlanEntry:    conv.NewEnterPlanPrompt(),
		},
	}
	m.userInput.Approval = input.NewApproval()

	m.mode.PlanApproval.Show(&tool.PlanRequest{Plan: "plan"}, "", 80, 24)
	m.userInput.Approval.Show(&perm.PermissionRequest{ToolName: "Bash", Description: "run"}, 80, 24)
	m.mode.Question.Show(&tool.QuestionRequest{}, 80)
	m.mode.PlanEntry.Show(&tool.EnterPlanRequest{}, 80)

	view := m.renderActiveModal("---", "")
	if !strings.Contains(view, "Would you like to proceed?") {
		t.Fatalf("expected plan approval modal to win priority, got %q", view)
	}
	if strings.Contains(view, "Do you want to proceed?") {
		t.Fatalf("expected approval modal to be hidden, got %q", view)
	}
}

func TestPermissionHookShowsPendingApprovalModal(t *testing.T) {
	engine := hook.NewEngine(setting.NewSettings(), "test-session", t.TempDir(), "")
	engine.AddSessionFunctionHook(hook.PermissionRequest, "", hook.FunctionHook{
		Callback: func(_ context.Context, _ hook.HookInput) (hook.HookOutput, error) {
			return hook.HookOutput{}, nil
		},
	})

	m := &model{
		width:  80,
		height: 24,
		runtime: appruntime.Model{
			HookEngine: engine,
		},
	}
	m.userInput.Approval = input.NewApproval()

	cmd := m.handlePermissionRequest(input.ApprovalRequestMsg{
		Request: &perm.PermissionRequest{ToolName: "Edit", FilePath: "/tmp/test.txt"},
	})

	if cmd == nil {
		t.Fatal("expected async hook command")
	}
	if !m.userInput.Approval.IsActive() {
		t.Fatal("expected approval modal to be active while hook runs")
	}
	view := m.userInput.Approval.Render()
	if !strings.Contains(view, "Do you want to proceed?") {
		t.Fatalf("expected normal approval modal while hook runs, got %q", view)
	}
	if strings.Contains(view, "Waiting for permission hook") {
		t.Fatalf("expected hook wait text to stay out of the foreground modal, got %q", view)
	}
}

func TestLatePermissionHookResultIsIgnoredAfterApprovalModalCloses(t *testing.T) {
	m := &model{}
	m.userInput.Approval = input.NewApproval()
	req := &perm.PermissionRequest{
		ID:       "perm-1",
		ToolName: "Edit",
		FilePath: "/tmp/test.txt",
	}
	m.userInput.Approval.Show(req, 80, 24)
	m.userInput.Approval.Hide()

	cmd := m.handleHookPermissionResult(hookPermissionResultMsg{
		Request: req,
		Allowed: true,
	})
	if cmd != nil {
		t.Fatal("expected stale hook result to be ignored")
	}
}

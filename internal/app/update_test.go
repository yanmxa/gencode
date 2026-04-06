package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	appskill "github.com/yanmxa/gencode/internal/app/skill"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/permission"
)

type fakeConversationRuntime struct {
	suggestCalled bool
	startCalled   bool
	lastStreamReq streamRequest
	streamResult  streamStartResult
}

func (f *fakeConversationRuntime) SuggestPromptCmd(req promptSuggestionRequest) tea.Cmd {
	f.suggestCalled = true
	return func() tea.Msg { return promptSuggestionMsg{text: "next"} }
}

func (f *fakeConversationRuntime) FetchTokenLimitsCmd(tokenLimitFetchRequest) tea.Cmd {
	return nil
}

func (f *fakeConversationRuntime) CompactCmd(compactRequest) tea.Cmd {
	return nil
}

func (f *fakeConversationRuntime) StartStream(req streamRequest) streamStartResult {
	f.startCalled = true
	f.lastStreamReq = req
	if f.streamResult.Cancel == nil {
		f.streamResult.Cancel = func() {}
	}
	if f.streamResult.Ch == nil {
		ch := make(chan message.StreamChunk)
		close(ch)
		f.streamResult.Ch = ch
	}
	return f.streamResult
}

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

func TestHandleQuestionResponse_ForAgentReplyChannel(t *testing.T) {
	reply := make(chan *tool.QuestionResponse, 1)
	m := &model{
		mode: appmode.State{
			Question:             appmode.NewQuestionPrompt(),
			PendingQuestion:      &tool.QuestionRequest{ID: "ask-1"},
			PendingQuestionReply: reply,
		},
	}

	resp := &tool.QuestionResponse{
		RequestID: "ask-1",
		Answers: map[int][]string{
			0: {"Patch"},
		},
	}
	cmd := m.handleQuestionResponse(appmode.QuestionResponseMsg{
		Request:  &tool.QuestionRequest{ID: "ask-1"},
		Response: resp,
	})

	if cmd != nil {
		t.Fatal("expected no follow-up command for agent question response")
	}
	if m.mode.PendingQuestion != nil {
		t.Fatal("expected pending question to be cleared")
	}
	if m.mode.PendingQuestionReply != nil {
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
// appears in the final system prompt when set on the System struct.
func TestSessionSummary_InSystemPrompt(t *testing.T) {
	summary := "Refactored the session package. Added overflow storage."

	sys := &system.System{
		Cwd:            "/tmp",
		SessionSummary: "<session-summary>\n" + summary + "\n</session-summary>",
	}
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
	sys := &system.System{
		Cwd:            "/tmp",
		SessionSummary: "",
	}
	prompt := sys.Prompt()

	if strings.Contains(prompt, "<session-summary>") {
		t.Error("system prompt should not contain <session-summary> when SessionSummary is empty")
	}
}

func TestReadSubmitRequest(t *testing.T) {
	t.Run("ignores submit while stream active", func(t *testing.T) {
		m := &model{conv: appconv.Model{Stream: appconv.StreamState{Active: true}}}
		if _, ok := m.readSubmitRequest(); ok {
			t.Fatal("expected no submit request while streaming")
		}
	})

	t.Run("ignores empty input without images", func(t *testing.T) {
		m := &model{input: newBaseModel("/tmp", modelInfra{}).input}
		m.input.Textarea.SetValue("   ")
		if _, ok := m.readSubmitRequest(); ok {
			t.Fatal("expected empty input to be ignored")
		}
	})

	t.Run("accepts image-only submit", func(t *testing.T) {
		m := &model{input: newBaseModel("/tmp", modelInfra{}).input}
		m.input.Images.Pending = []message.ImageData{{FileName: "a.png"}}
		req, ok := m.readSubmitRequest()
		if !ok {
			t.Fatal("expected image-only submit to be accepted")
		}
		if req.Input != "" {
			t.Fatalf("expected empty text input, got %q", req.Input)
		}
	})
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
		"*provider.Model",
		"*tool.Model",
		"*skill.Model",
		"*agent.Model",
		"*mcp.Model",
		"*plugin.Model",
		"*session.Model",
		"*memory.Model",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected selector order:\n got: %v\nwant: %v", got, want)
	}
}

func TestStartPromptSuggestionUsesRuntimeInterface(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		runtime: rt,
		loop:    &core.Loop{Client: &client.Client{}},
		conv: appconv.Model{
			Messages: []message.ChatMessage{
				{Role: message.RoleAssistant, Content: "first"},
				{Role: message.RoleAssistant, Content: "second"},
			},
		},
	}

	cmd := m.startPromptSuggestion()
	if cmd == nil {
		t.Fatal("expected prompt suggestion command")
	}
	if !rt.suggestCalled {
		t.Fatal("expected runtime suggestion command to be used")
	}
}

func TestStartLLMStreamUsesRuntimeInterface(t *testing.T) {
	rt := &fakeConversationRuntime{}
	m := &model{
		runtime: rt,
		loop:    &core.Loop{},
		conv: appconv.Model{
			Messages: []message.ChatMessage{
				{Role: message.RoleUser, Content: "hello"},
			},
		},
		provider: appprovider.State{
			ThinkingOverride: provider.ThinkingOff,
		},
	}

	cmd := m.startLLMStream(nil)
	if cmd == nil {
		t.Fatal("expected stream command")
	}
	if !rt.startCalled {
		t.Fatal("expected runtime start stream to be used")
	}
	if len(rt.lastStreamReq.Messages) != 1 {
		t.Fatalf("runtime should receive committed conversation only, got %d messages", len(rt.lastStreamReq.Messages))
	}
	if rt.lastStreamReq.Messages[0].Content != "hello" {
		t.Fatalf("unexpected request messages: %#v", rt.lastStreamReq.Messages)
	}
	if len(m.conv.Messages) != 2 {
		t.Fatalf("expected assistant placeholder to be appended after stream start, got %d messages", len(m.conv.Messages))
	}
	if m.conv.Messages[1].Role != message.RoleAssistant || m.conv.Messages[1].Content != "" {
		t.Fatalf("unexpected assistant placeholder: %#v", m.conv.Messages[1])
	}
	if !m.conv.Stream.Active {
		t.Fatal("expected stream to be marked active")
	}
	if m.conv.Stream.Cancel == nil {
		t.Fatal("expected stream cancel func to be set")
	}
}

func TestBuildStreamRequestExcludesAssistantPlaceholder(t *testing.T) {
	rt := &fakeConversationRuntime{
		streamResult: streamStartResult{
			Cancel: context.CancelFunc(func() {}),
		},
	}
	ch := make(chan message.StreamChunk)
	close(ch)
	rt.streamResult.Ch = ch
	m := &model{
		runtime: rt,
		loop:    &core.Loop{},
		conv: appconv.Model{
			Messages: []message.ChatMessage{
				{Role: message.RoleUser, Content: "user"},
				{Role: message.RoleAssistant, Content: "assistant"},
			},
		},
	}

	_ = m.startContinueStream()

	if len(rt.lastStreamReq.Messages) != 2 {
		t.Fatalf("expected 2 provider messages before placeholder append, got %d", len(rt.lastStreamReq.Messages))
	}
	if len(m.conv.Messages) != 3 {
		t.Fatalf("expected placeholder append after request build, got %d total messages", len(m.conv.Messages))
	}
}

func TestBuildPromptSuggestionRequest(t *testing.T) {
	m := &model{
		loop: &core.Loop{Client: &client.Client{}},
		conv: appconv.Model{
			Messages: []message.ChatMessage{
				{Role: message.RoleUser, Content: "u1"},
				{Role: message.RoleAssistant, Content: "a1"},
				{Role: message.RoleAssistant, Content: "a2"},
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
	if last.Role != message.RoleUser || last.Content != suggestionUserPrompt {
		t.Fatalf("unexpected tail message: %#v", last)
	}
}

func TestBuildCompactRequest(t *testing.T) {
	m := &model{
		loop: &core.Loop{},
		session: appsession.State{
			Summary: "existing summary",
		},
		conv: appconv.Model{
			Messages: []message.ChatMessage{
				{Role: message.RoleUser, Content: "hello"},
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

func TestBuildLoopExtraIncludesSkillInvocationAndTaskReminder(t *testing.T) {
	tool.DefaultTodoStore.Reset()
	t.Cleanup(tool.DefaultTodoStore.Reset)
	tool.DefaultTodoStore.Create("Write tests", "Cover loop builder", "Writing tests", nil)

	m := &model{
		skill: appskill.State{
			ActiveInvocation: "<skill>Use the active skill</skill>",
		},
		conv: appconv.Model{
			TurnsSinceLastTaskTool: taskReminderThreshold,
		},
	}

	extra := m.buildLoopExtra([]string{"base"})
	if len(extra) != 3 {
		t.Fatalf("expected 3 extra entries, got %d: %#v", len(extra), extra)
	}
	if extra[0] != "base" || extra[1] != "<skill>Use the active skill</skill>" {
		t.Fatalf("unexpected extra ordering: %#v", extra)
	}
	if !strings.Contains(extra[2], "<task-reminder>") || !strings.Contains(extra[2], "Write tests") {
		t.Fatalf("expected task reminder entry, got %#v", extra[2])
	}
}

func TestBuildLoopSystemIncludesSessionSummary(t *testing.T) {
	m := &model{
		cwd: "/tmp/project",
		session: appsession.State{
			Summary: "condensed summary",
		},
		memory: appmemory.State{
			CachedUser:    "user memory",
			CachedProject: "project memory",
		},
	}

	sys := m.buildLoopSystem([]string{"extra"}, &client.Client{})
	if sys.Cwd != "/tmp/project" {
		t.Fatalf("unexpected cwd: %q", sys.Cwd)
	}
	if sys.SessionSummary != "<session-summary>\ncondensed summary\n</session-summary>" {
		t.Fatalf("unexpected session summary block: %q", sys.SessionSummary)
	}
	if len(sys.Extra) != 1 || sys.Extra[0] != "extra" {
		t.Fatalf("unexpected extra: %#v", sys.Extra)
	}
	if sys.UserInstructions != "user memory" || sys.ProjectInstructions != "project memory" {
		t.Fatalf("unexpected memory context: user=%q project=%q", sys.UserInstructions, sys.ProjectInstructions)
	}
}

func TestConfigureLoopBuildsLoopComponents(t *testing.T) {
	m := &model{
		cwd:  "/tmp/project",
		loop: &core.Loop{},
		provider: appprovider.State{
			ThinkingLevel:    provider.ThinkingNormal,
			ThinkingOverride: provider.ThinkingHigh,
		},
		mode: appmode.State{
			Enabled:       true,
			DisabledTools: map[string]bool{"Bash": true},
		},
		memory: appmemory.State{
			CachedUser:    "user memory",
			CachedProject: "project memory",
		},
		session: appsession.State{
			Summary: "session summary",
		},
	}

	m.configureLoop([]string{"explicit-extra"})

	if m.loop.Client == nil || m.loop.System == nil || m.loop.Tool == nil {
		t.Fatal("configureLoop should populate loop client/system/tool")
	}
	if m.loop.Client.ThinkingLevel != provider.ThinkingHigh {
		t.Fatalf("unexpected thinking level: %v", m.loop.Client.ThinkingLevel)
	}
	if !m.loop.Tool.PlanMode {
		t.Fatal("expected plan mode on tool set")
	}
	if !m.loop.Tool.Disabled["Bash"] {
		t.Fatalf("expected disabled tools to propagate: %#v", m.loop.Tool.Disabled)
	}
	if m.loop.System.SessionSummary != "<session-summary>\nsession summary\n</session-summary>" {
		t.Fatalf("unexpected session summary: %q", m.loop.System.SessionSummary)
	}
	if len(m.loop.System.Extra) != 1 || m.loop.System.Extra[0] != "explicit-extra" {
		t.Fatalf("unexpected system extra: %#v", m.loop.System.Extra)
	}
}

func TestDetectThinkingKeywords(t *testing.T) {
	t.Run("high thinking keywords", func(t *testing.T) {
		m := &model{}
		m.detectThinkingKeywords("Please think carefully before answering")
		if m.provider.ThinkingOverride != provider.ThinkingHigh {
			t.Fatalf("expected high thinking override, got %v", m.provider.ThinkingOverride)
		}
	})

	t.Run("ultra keywords win over high", func(t *testing.T) {
		m := &model{}
		m.detectThinkingKeywords("Think hard and ultrathink about this")
		if m.provider.ThinkingOverride != provider.ThinkingUltra {
			t.Fatalf("expected ultra thinking override, got %v", m.provider.ThinkingOverride)
		}
	})
}

func TestRenderActiveModalPriority(t *testing.T) {
	m := &model{
		mode: appmode.State{
			PlanApproval: appmode.NewPlanPrompt(),
			Question:     appmode.NewQuestionPrompt(),
			PlanEntry:    appmode.NewEnterPlanPrompt(),
		},
		approval: appapproval.New(),
	}

	m.mode.PlanApproval.Show(&tool.PlanRequest{Plan: "plan"}, "", 80, 24)
	m.approval.Show(&permission.PermissionRequest{ToolName: "Bash", Description: "run"}, 80, 24)
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
	engine := hooks.NewEngine(config.NewSettings(), "test-session", t.TempDir(), "")
	engine.AddSessionFunctionHook(hooks.PermissionRequest, "", hooks.FunctionHook{
		Callback: func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
			return hooks.HookOutput{}, nil
		},
	})

	m := &model{
		approval:   appapproval.New(),
		width:      80,
		height:     24,
		hookEngine: engine,
	}

	cmd := m.handlePermissionRequest(appapproval.RequestMsg{
		Request: &permission.PermissionRequest{ToolName: "Edit", FilePath: "/tmp/test.txt"},
	})

	if cmd == nil {
		t.Fatal("expected async hook command")
	}
	if !m.approval.IsActive() {
		t.Fatal("expected approval modal to be active while hook runs")
	}
	view := m.approval.Render()
	if !strings.Contains(view, "Do you want to proceed?") {
		t.Fatalf("expected normal approval modal while hook runs, got %q", view)
	}
	if strings.Contains(view, "Waiting for permission hook") {
		t.Fatalf("expected hook wait text to stay out of the foreground modal, got %q", view)
	}
}

func TestLatePermissionHookResultIsIgnoredAfterApprovalModalCloses(t *testing.T) {
	m := &model{
		approval: appapproval.New(),
	}
	req := &permission.PermissionRequest{
		ID:       "perm-1",
		ToolName: "Edit",
		FilePath: "/tmp/test.txt",
	}
	m.approval.Show(req, 80, 24)
	m.approval.Hide()

	cmd := m.handleHookPermissionResult(hookPermissionResultMsg{
		Request: req,
		Allowed: true,
	})
	if cmd != nil {
		t.Fatal("expected stale hook result to be ignored")
	}
}

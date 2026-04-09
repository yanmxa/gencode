package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	_ "github.com/yanmxa/gencode/internal/tool/registry"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// --- Test helpers ---

// mockProvider implements provider.LLMProvider for testing.
type mockProvider struct {
	responses []message.CompletionResponse
	callIdx   int
}

func (m *mockProvider) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk, 1)
	go func() {
		defer close(ch)
		if m.callIdx >= len(m.responses) {
			ch <- message.StreamChunk{Type: message.ChunkTypeDone, Response: &message.CompletionResponse{
				Content:    "no more responses",
				StopReason: "end_turn",
			}}
			return
		}
		resp := m.responses[m.callIdx]
		m.callIdx++
		ch <- message.StreamChunk{Type: message.ChunkTypeDone, Response: &resp}
	}()
	return ch
}

func (m *mockProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (m *mockProvider) Name() string { return "mock" }

type hookResponseTestTool struct{}

func (t *hookResponseTestTool) Name() string        { return "CoreHookResponseTestTool" }
func (t *hookResponseTestTool) Description() string { return "test tool" }
func (t *hookResponseTestTool) Icon() string        { return "t" }
func (t *hookResponseTestTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return toolresult.ToolResult{
		Success:      true,
		Output:       "ok",
		HookResponse: map[string]any{"status": "structured"},
	}
}

// newTestLoop creates a Loop with the new struct layout for testing.
func newTestLoop(mp provider.LLMProvider) *Loop {
	c := &client.Client{Provider: mp, Model: "test-model", MaxTokens: 8192}
	return &Loop{
		System:     &system.System{Client: c, Cwd: "/tmp"},
		Client:     c,
		Tool:       &tool.Set{},
		Permission: permission.PermitAll(),
	}
}

// --- Tests ---

func TestLoopInit(t *testing.T) {
	loop := newTestLoop(&mockProvider{})
	if loop == nil {
		t.Fatal("loop is nil")
	}
	if len(loop.Messages()) != 0 {
		t.Errorf("expected 0 messages, got %d", len(loop.Messages()))
	}
}

func TestAddUser(t *testing.T) {
	loop := &Loop{}

	loop.AddUser("hello", nil)
	msgs := loop.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != message.RoleUser {
		t.Errorf("expected role 'user', got '%s'", msgs[0].Role)
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected content 'hello', got '%s'", msgs[0].Content)
	}
}

func TestAddUserWithImages(t *testing.T) {
	loop := &Loop{}

	images := []message.ImageData{
		{MediaType: "image/png", Data: "abc123", FileName: "test.png", Size: 100},
	}
	loop.AddUser("hello", images)
	msgs := loop.Messages()
	if len(msgs[0].Images) != 1 {
		t.Errorf("expected 1 image, got %d", len(msgs[0].Images))
	}
}

func TestAddResponse(t *testing.T) {
	loop := &Loop{Client: &client.Client{Provider: &mockProvider{}}}

	resp := &message.CompletionResponse{
		Content: "response text",
		Usage:   message.Usage{InputTokens: 100, OutputTokens: 50},
		ToolCalls: []message.ToolCall{
			{ID: "tc1", Name: "Read", Input: `{"file_path": "/tmp/test"}`},
		},
	}

	calls := loop.AddResponse(resp)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "Read" {
		t.Errorf("expected tool 'Read', got '%s'", calls[0].Name)
	}

	tokens := loop.Tokens()
	if tokens.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", tokens.InputTokens)
	}
	if tokens.OutputTokens != 50 {
		t.Errorf("expected output tokens 50, got %d", tokens.OutputTokens)
	}
	if tokens.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", tokens.TotalTokens)
	}

	msgs := loop.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != message.RoleAssistant {
		t.Errorf("expected role 'assistant', got '%s'", msgs[0].Role)
	}
}

func TestAddToolResult(t *testing.T) {
	loop := &Loop{}

	r := message.ToolResult{
		ToolCallID: "tc1",
		ToolName:   "Read",
		Content:    "file content here",
	}
	loop.AddToolResult(r)

	msgs := loop.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != message.RoleUser {
		t.Errorf("expected role 'user', got '%s'", msgs[0].Role)
	}
	if msgs[0].ToolResult == nil {
		t.Fatal("expected tool result, got nil")
	}
	if msgs[0].ToolResult.Content != "file content here" {
		t.Errorf("expected content 'file content here', got '%s'", msgs[0].ToolResult.Content)
	}
}

func TestExecToolPreservesHookResponse(t *testing.T) {
	tool.Register(&hookResponseTestTool{})

	loop := newTestLoop(&mockProvider{})
	result := loop.ExecTool(context.Background(), message.ToolCall{
		ID:    "tc-hook",
		Name:  "CoreHookResponseTestTool",
		Input: `{}`,
	})

	if result == nil {
		t.Fatal("expected tool result")
	}
	if result.IsError {
		t.Fatalf("expected successful tool result, got error: %s", result.Content)
	}
	response, ok := result.HookResponse.(map[string]any)
	if !ok {
		t.Fatalf("expected structured hook response, got %T", result.HookResponse)
	}
	if response["status"] != "structured" {
		t.Fatalf("unexpected hook response: %#v", response)
	}
}

func TestExecToolRoutesAskUserQuestionThroughQuestionHandler(t *testing.T) {
	loop := newTestLoop(&mockProvider{})
	loop.QuestionHandler = func(ctx context.Context, req *tool.QuestionRequest) (*tool.QuestionResponse, error) {
		return &tool.QuestionResponse{
			RequestID: req.ID,
			Answers: map[int][]string{
				0: {"Patch"},
			},
		}, nil
	}

	result := loop.ExecTool(context.Background(), message.ToolCall{
		ID:    "tc-question",
		Name:  "AskUserQuestion",
		Input: `{"questions":[{"header":"Release","question":"What type of version bump would you like?","options":[{"label":"Patch"},{"label":"Minor"},{"label":"Major"}]}]}`,
	})

	if result == nil {
		t.Fatal("expected tool result")
	}
	if result.IsError {
		t.Fatalf("expected successful tool result, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Release: Patch") {
		t.Fatalf("expected question response in result, got %q", result.Content)
	}
}

func TestRunDrainsInjectedInputsBeforeNextTurn(t *testing.T) {
	tool.Register(&hookResponseTestTool{})

	loop := newTestLoop(&mockProvider{
		responses: []message.CompletionResponse{
			{
				Content:    "",
				StopReason: "tool_use",
				ToolCalls: []message.ToolCall{
					{ID: "tc-hook", Name: "CoreHookResponseTestTool", Input: `{}`},
				},
			},
			{
				Content:    "done",
				StopReason: "end_turn",
			},
		},
	})

	injected := false
	result, err := loop.Run(context.Background(), RunOptions{
		MaxTurns: 3,
		DrainInjectedInputs: func() []string {
			if injected {
				return nil
			}
			injected = true
			return []string{"Please incorporate the latest reviewer feedback."}
		},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, StopEndTurn)
	}

	found := false
	for _, msg := range result.Messages {
		if msg.Role == message.RoleUser && msg.Content == "Please incorporate the latest reviewer feedback." {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected injected user message to be added before the next turn")
	}
}

func TestSetMessages(t *testing.T) {
	loop := &Loop{}

	msgs := []message.Message{
		{Role: message.RoleUser, Content: "hello"},
		{Role: message.RoleAssistant, Content: "world"},
	}
	loop.SetMessages(msgs)

	got := loop.Messages()
	if len(got) != 2 {
		t.Errorf("expected 2 messages, got %d", len(got))
	}
}

func TestDecideCompletion(t *testing.T) {
	t.Run("recover truncated response without tools", func(t *testing.T) {
		decision := DecideCompletion("max_tokens", nil, 0, DefaultMaxOutputRecovery)
		if decision.Action != CompletionRecoverMaxTokens {
			t.Fatalf("expected CompletionRecoverMaxTokens, got %v", decision.Action)
		}
	})

	t.Run("stop after recovery exhausted", func(t *testing.T) {
		decision := DecideCompletion("max_tokens", nil, DefaultMaxOutputRecovery, DefaultMaxOutputRecovery)
		if decision.Action != CompletionStopMaxOutputRecovery {
			t.Fatalf("expected CompletionStopMaxOutputRecovery, got %v", decision.Action)
		}
	})

	t.Run("run tools when tool calls exist", func(t *testing.T) {
		calls := []message.ToolCall{{ID: "tc1", Name: "Read"}}
		decision := DecideCompletion("tool_use", calls, 0, DefaultMaxOutputRecovery)
		if decision.Action != CompletionRunTools {
			t.Fatalf("expected CompletionRunTools, got %v", decision.Action)
		}
		if len(decision.ToolCalls) != 1 || decision.ToolCalls[0].Name != "Read" {
			t.Fatalf("unexpected tool calls: %+v", decision.ToolCalls)
		}
	})

	t.Run("end turn when no tools and not truncated", func(t *testing.T) {
		decision := DecideCompletion("end_turn", nil, 0, DefaultMaxOutputRecovery)
		if decision.Action != CompletionEndTurn {
			t.Fatalf("expected CompletionEndTurn, got %v", decision.Action)
		}
	})
}

func TestCanCompactMessages(t *testing.T) {
	if CanCompactMessages(2) {
		t.Fatal("expected short conversation to skip compaction")
	}
	if !CanCompactMessages(3) {
		t.Fatal("expected 3-message conversation to allow compaction")
	}
}

func TestShouldCompactPromptTooLong(t *testing.T) {
	if ShouldCompactPromptTooLong(nil, 10) {
		t.Fatal("nil error should not trigger compaction")
	}
	if ShouldCompactPromptTooLong(errors.New("prompt is too long"), 2) {
		t.Fatal("too-short conversation should not trigger compaction")
	}
	if !ShouldCompactPromptTooLong(errors.New("prompt_too_long"), 3) {
		t.Fatal("expected prompt-too-long with enough messages to trigger compaction")
	}
}

func TestIsPromptTooLong(t *testing.T) {
	if !IsPromptTooLong(errors.New("prompt is too long for this model")) {
		t.Fatal("expected natural language prompt-too-long error to match")
	}
	if !IsPromptTooLong(errors.New("provider returned prompt_too_long")) {
		t.Fatal("expected provider prompt_too_long error to match")
	}
	if IsPromptTooLong(errors.New("rate limit exceeded")) {
		t.Fatal("expected unrelated error not to match")
	}
}

func TestDecisionConstants(t *testing.T) {
	if permission.Permit != 0 {
		t.Error("Permit should be 0")
	}
	if permission.Reject != 1 {
		t.Error("Reject should be 1")
	}
	if permission.Prompt != 2 {
		t.Error("Prompt should be 2")
	}
}

func TestPermitAll(t *testing.T) {
	auth := permission.PermitAll()
	if auth.Check("Bash", nil) != permission.Permit {
		t.Error("PermitAll should always return Permit")
	}
	if auth.Check("Write", map[string]any{"file": "x"}) != permission.Permit {
		t.Error("PermitAll should always return Permit")
	}
}

func TestReadOnly(t *testing.T) {
	auth := permission.ReadOnly()
	if auth.Check("Read", nil) != permission.Permit {
		t.Error("ReadOnly should permit Read")
	}
	if auth.Check("Glob", nil) != permission.Permit {
		t.Error("ReadOnly should permit Glob")
	}
	if auth.Check("Grep", nil) != permission.Permit {
		t.Error("ReadOnly should permit Grep")
	}
	if auth.Check("Write", nil) != permission.Reject {
		t.Error("ReadOnly should reject Write")
	}
	if auth.Check("Bash", nil) != permission.Reject {
		t.Error("ReadOnly should reject Bash")
	}
	if auth.Check("Edit", nil) != permission.Reject {
		t.Error("ReadOnly should reject Edit")
	}
}

func TestDenyAll(t *testing.T) {
	auth := permission.DenyAll()
	if auth.Check("Read", nil) != permission.Reject {
		t.Error("DenyAll should always return Reject")
	}
}

func TestIsReadOnlyTool(t *testing.T) {
	if !permission.IsReadOnlyTool("Read") {
		t.Error("Read should be read-only")
	}
	if !permission.IsReadOnlyTool("Glob") {
		t.Error("Glob should be read-only")
	}
	if !permission.IsReadOnlyTool("Grep") {
		t.Error("Grep should be read-only")
	}
	if !permission.IsReadOnlyTool("WebFetch") {
		t.Error("WebFetch should be read-only")
	}
	if !permission.IsReadOnlyTool("WebSearch") {
		t.Error("WebSearch should be read-only")
	}
	if !permission.IsReadOnlyTool("LSP") {
		t.Error("LSP should be read-only")
	}
	if permission.IsReadOnlyTool("Bash") {
		t.Error("Bash should not be read-only")
	}
	if permission.IsReadOnlyTool("Write") {
		t.Error("Write should not be read-only")
	}
}

func TestCollect(t *testing.T) {
	ctx := context.Background()

	ch := make(chan message.StreamChunk, 5)
	ch <- message.StreamChunk{Type: message.ChunkTypeText, Text: "hello "}
	ch <- message.StreamChunk{Type: message.ChunkTypeThinking, Text: "thinking..."}
	ch <- message.StreamChunk{Type: message.ChunkTypeText, Text: "world"}
	ch <- message.StreamChunk{Type: message.ChunkTypeDone, Response: &message.CompletionResponse{
		Content:    "hello world",
		Thinking:   "thinking...",
		StopReason: "end_turn",
		Usage:      message.Usage{InputTokens: 10, OutputTokens: 5},
	}}
	close(ch)

	resp, err := Collect(ctx, ch)
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}
	if resp.Content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", resp.Content)
	}
}

func TestCollectError(t *testing.T) {
	ctx := context.Background()

	ch := make(chan message.StreamChunk, 2)
	ch <- message.StreamChunk{Type: message.ChunkTypeError, Error: context.DeadlineExceeded}
	close(ch)

	_, err := Collect(ctx, ch)
	if err == nil {
		t.Fatal("Collect() should return error")
	}
}

func TestCollectWithToolCalls(t *testing.T) {
	ctx := context.Background()

	ch := make(chan message.StreamChunk, 5)
	ch <- message.StreamChunk{Type: message.ChunkTypeToolStart, ToolID: "t1", ToolName: "Read"}
	ch <- message.StreamChunk{Type: message.ChunkTypeToolInput, Text: `{"file_path": "/tmp"}`}
	ch <- message.StreamChunk{Type: message.ChunkTypeDone}
	close(ch)

	resp, err := Collect(ctx, ch)
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "Read" {
		t.Errorf("expected tool 'Read', got '%s'", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Input != `{"file_path": "/tmp"}` {
		t.Errorf("unexpected input: %s", resp.ToolCalls[0].Input)
	}
}

func TestFilterToolCallsNoHooks(t *testing.T) {
	loop := &Loop{}
	calls := []message.ToolCall{
		{ID: "t1", Name: "Read"},
		{ID: "t2", Name: "Write"},
	}

	allowed, blocked, _, _ := loop.FilterToolCalls(context.Background(), calls)
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed, got %d", len(allowed))
	}
	if len(blocked) != 0 {
		t.Errorf("expected 0 blocked, got %d", len(blocked))
	}
}

// --- State machine tests ---

func TestRunMaxOutputRecovery(t *testing.T) {
	// Provider returns max_tokens twice, then end_turn.
	// The loop should inject resume messages and succeed.
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "partial1", StopReason: "max_tokens", Usage: message.Usage{InputTokens: 10, OutputTokens: 100}},
			{Content: "partial2", StopReason: "max_tokens", Usage: message.Usage{InputTokens: 20, OutputTokens: 100}},
			{Content: "done", StopReason: "end_turn", Usage: message.Usage{InputTokens: 30, OutputTokens: 50}},
		},
	}

	loop := newTestLoop(mp)
	loop.AddUser("hello", nil)

	result, err := loop.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopEndTurn {
		t.Errorf("expected stop reason %q, got %q", StopEndTurn, result.StopReason)
	}
	if result.Content != "done" {
		t.Errorf("expected content 'done', got %q", result.Content)
	}
	recoveryCount := 0
	for _, tr := range result.Transitions {
		if tr == TransitionMaxOutputRecovery {
			recoveryCount++
		}
	}
	if recoveryCount != 2 {
		t.Errorf("expected 2 recovery transitions, got %d (transitions: %v)", recoveryCount, result.Transitions)
	}
}

func TestRunMaxOutputRecoveryExhausted(t *testing.T) {
	mp := &mockProvider{}
	for i := 0; i < 5; i++ {
		mp.responses = append(mp.responses, message.CompletionResponse{
			Content:    "truncated",
			StopReason: "max_tokens",
			Usage:      message.Usage{InputTokens: 10, OutputTokens: 100},
		})
	}

	loop := newTestLoop(mp)
	loop.AddUser("hello", nil)

	result, err := loop.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopMaxOutputRecoveryExhausted {
		t.Errorf("expected stop reason %q, got %q", StopMaxOutputRecoveryExhausted, result.StopReason)
	}
}

func TestRunPromptTooLongRecovery(t *testing.T) {
	mp := &errorThenSuccessProvider{
		errMsg:      "prompt is too long",
		successResp: message.CompletionResponse{Content: "recovered", StopReason: "end_turn", Usage: message.Usage{InputTokens: 5, OutputTokens: 5}},
	}

	loop := newTestLoop(mp)
	loop.AddUser("hello", nil)
	loop.AddUser("msg2", nil)
	loop.AddUser("msg3", nil)

	result, err := loop.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopEndTurn {
		t.Errorf("expected stop reason %q, got %q", StopEndTurn, result.StopReason)
	}
	// Should have a prompt-too-long transition
	found := false
	for _, tr := range result.Transitions {
		if tr == TransitionPromptTooLong {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TransitionPromptTooLong in transitions: %v", result.Transitions)
	}
}

func TestRunTransitions(t *testing.T) {
	// Multi-turn with tool calls: verify transition log.
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "", StopReason: "tool_use", ToolCalls: []message.ToolCall{{ID: "t1", Name: "UnknownTool", Input: "{}"}}, Usage: message.Usage{InputTokens: 1, OutputTokens: 1}},
			{Content: "done", StopReason: "end_turn", Usage: message.Usage{InputTokens: 2, OutputTokens: 2}},
		},
	}

	loop := newTestLoop(mp)
	loop.AddUser("go", nil)

	result, err := loop.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(result.Transitions) != 2 {
		t.Errorf("expected 2 transitions, got %d: %v", len(result.Transitions), result.Transitions)
	}
	if result.Transitions[0] != TransitionNextTurn {
		t.Errorf("expected first transition 'next_turn', got %q", result.Transitions[0])
	}
	if result.Transitions[1] != TransitionNextTurn {
		t.Errorf("expected second transition 'next_turn', got %q", result.Transitions[1])
	}
}

// errorThenSuccessProvider returns an error on the first Stream call, then succeeds.
type errorThenSuccessProvider struct {
	errMsg      string
	successResp message.CompletionResponse
	callCount   int
}

func (p *errorThenSuccessProvider) Stream(ctx context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
	ch := make(chan message.StreamChunk, 1)
	go func() {
		defer close(ch)
		p.callCount++
		if p.callCount == 1 {
			ch <- message.StreamChunk{Type: message.ChunkTypeError, Error: errors.New(p.errMsg)}
			return
		}
		ch <- message.StreamChunk{Type: message.ChunkTypeDone, Response: &p.successResp}
	}()
	return ch
}

func (p *errorThenSuccessProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (p *errorThenSuccessProvider) Name() string { return "error-then-success" }

func TestRunEndTurn(t *testing.T) {
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "done", StopReason: "end_turn", Usage: message.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}

	loop := newTestLoop(mp)
	loop.AddUser("hello", nil)

	result, err := loop.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopEndTurn {
		t.Errorf("expected stop reason %q, got %q", StopEndTurn, result.StopReason)
	}
	if result.Content != "done" {
		t.Errorf("expected content 'done', got %q", result.Content)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	if result.Tokens.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", result.Tokens.InputTokens)
	}
}

func TestRunMaxTurns(t *testing.T) {
	mp := &mockProvider{}
	for i := 0; i < 5; i++ {
		mp.responses = append(mp.responses, message.CompletionResponse{
			Content:    "",
			StopReason: "tool_use",
			ToolCalls: []message.ToolCall{
				{ID: "tc", Name: "UnknownTool", Input: "{}"},
			},
			Usage: message.Usage{InputTokens: 1, OutputTokens: 1},
		})
	}

	loop := newTestLoop(mp)
	loop.AddUser("go", nil)

	result, err := loop.Run(context.Background(), RunOptions{MaxTurns: 3})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.StopReason != StopMaxTurns {
		t.Errorf("expected stop reason 'max_turns', got '%s'", result.StopReason)
	}
}

func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "done", StopReason: "end_turn"},
		},
	}

	loop := newTestLoop(mp)
	loop.AddUser("hello", nil)

	result, err := loop.Run(ctx, RunOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if result.StopReason != StopCancelled {
		t.Errorf("expected stop reason 'cancelled', got '%s'", result.StopReason)
	}
}

func TestStaticTools(t *testing.T) {
	tools := []provider.ToolSchema{
		{Name: "Read", Description: "Read files"},
		{Name: "Write", Description: "Write files"},
	}
	st := &tool.Set{Static: tools}
	got := st.Tools()
	if len(got) != 2 {
		t.Errorf("expected 2 tools, got %d", len(got))
	}
}

func TestLoopClientAccess(t *testing.T) {
	c := &client.Client{Provider: &mockProvider{}, Model: "model-a"}
	loop := &Loop{Client: c}
	if loop.Client.Model != "model-a" {
		t.Errorf("expected model-a, got %s", loop.Client.Model)
	}

	loop.Client.Model = "model-b"
	if loop.Client.Model != "model-b" {
		t.Errorf("expected model-b, got %s", loop.Client.Model)
	}
}

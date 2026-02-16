package core

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
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

	allowed, blocked := loop.FilterToolCalls(context.Background(), calls)
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed, got %d", len(allowed))
	}
	if len(blocked) != 0 {
		t.Errorf("expected 0 blocked, got %d", len(blocked))
	}
}

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
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop reason 'end_turn', got '%s'", result.StopReason)
	}
	if result.Content != "done" {
		t.Errorf("expected content 'done', got '%s'", result.Content)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	if result.Tokens.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", result.Tokens.InputTokens)
	}
}

func TestRunMaxTurns(t *testing.T) {
	// Provider always returns tool calls, forcing the loop to hit max turns.
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
	if result.StopReason != "max_turns" {
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
	if result.StopReason != "cancelled" {
		t.Errorf("expected stop reason 'cancelled', got '%s'", result.StopReason)
	}
}

func TestStaticTools(t *testing.T) {
	tools := []provider.Tool{
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

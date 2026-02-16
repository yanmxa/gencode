package loop_test

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

func TestLoop_SingleTurn_EndTurn(t *testing.T) {
	loop, _ := testutil.NewTestLoop(t,
		testutil.EndTurnResponse("hello world"),
	)
	loop.AddUser("hi", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.StopReason != "end_turn" {
		t.Errorf("expected stop reason 'end_turn', got %q", result.StopReason)
	}
	if result.Content != "hello world" {
		t.Errorf("expected content 'hello world', got %q", result.Content)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	if result.Tokens.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
}

func TestLoop_MultiTurn_ToolUse(t *testing.T) {
	testutil.RegisterFakeTool(t, "MyTool", "tool output")

	loop, _ := testutil.NewTestLoop(t,
		testutil.ToolCallResponse("MyTool", "tc1", `{}`),
		testutil.EndTurnResponse("done after tool"),
	)
	loop.AddUser("use tool", nil)

	var toolExecuted bool
	result, err := loop.Run(context.Background(), core.RunOptions{
		OnToolDone: func(tc message.ToolCall, r message.ToolResult) {
			toolExecuted = true
			if tc.Name != "MyTool" {
				t.Errorf("expected tool 'MyTool', got %q", tc.Name)
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !toolExecuted {
		t.Error("expected tool to be executed")
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", result.Turns)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", result.StopReason)
	}

	// Verify messages contain tool call and result
	msgs := result.Messages
	hasToolCall := false
	hasToolResult := false
	for _, m := range msgs {
		if m.Role == message.RoleAssistant && len(m.ToolCalls) > 0 {
			hasToolCall = true
		}
		if m.ToolResult != nil {
			hasToolResult = true
		}
	}
	if !hasToolCall {
		t.Error("expected tool call in messages")
	}
	if !hasToolResult {
		t.Error("expected tool result in messages")
	}
}

func TestLoop_MaxTurns(t *testing.T) {
	testutil.RegisterFakeTool(t, "AlwaysTool", "ok")

	// Queue enough tool-use responses to exceed max turns
	responses := make([]message.CompletionResponse, 10)
	for i := range responses {
		responses[i] = testutil.ToolCallResponse("AlwaysTool", "tc", `{}`)
	}

	loop, _ := testutil.NewTestLoop(t, responses...)
	loop.AddUser("go", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{MaxTurns: 3})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.StopReason != "max_turns" {
		t.Errorf("expected 'max_turns', got %q", result.StopReason)
	}
}

func TestLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	loop, _ := testutil.NewTestLoop(t,
		testutil.EndTurnResponse("should not reach"),
	)
	loop.AddUser("hello", nil)

	result, err := loop.Run(ctx, core.RunOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if result.StopReason != "cancelled" {
		t.Errorf("expected 'cancelled', got %q", result.StopReason)
	}
}

func TestLoop_UnknownTool(t *testing.T) {
	// LLM requests a tool that doesn't exist, then ends turn
	loop, _ := testutil.NewTestLoop(t,
		testutil.ToolCallResponse("NonExistent", "tc1", `{}`),
		testutil.EndTurnResponse("recovered"),
	)
	loop.AddUser("call unknown", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.StopReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", result.StopReason)
	}

	// Verify error result was added to conversation
	hasError := false
	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("expected error tool result for unknown tool")
	}
}

func TestLoop_MultipleToolCalls(t *testing.T) {
	testutil.RegisterFakeTool(t, "ToolA", "result A")
	testutil.RegisterFakeTool(t, "ToolB", "result B")

	loop, _ := testutil.NewTestLoop(t,
		testutil.MultiToolCallResponse(
			message.ToolCall{ID: "tc1", Name: "ToolA", Input: `{}`},
			message.ToolCall{ID: "tc2", Name: "ToolB", Input: `{}`},
		),
		testutil.EndTurnResponse("both done"),
	)
	loop.AddUser("use both", nil)

	var toolsDone int
	result, err := loop.Run(context.Background(), core.RunOptions{
		OnToolDone: func(tc message.ToolCall, r message.ToolResult) {
			toolsDone++
		},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if toolsDone != 2 {
		t.Errorf("expected 2 tools executed, got %d", toolsDone)
	}

	// Verify both results are in messages
	toolResults := 0
	for _, m := range result.Messages {
		if m.ToolResult != nil && !m.ToolResult.IsError {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Errorf("expected 2 tool results, got %d", toolResults)
	}
}

func TestLoop_TokenAccumulation(t *testing.T) {
	testutil.RegisterFakeTool(t, "Tick", "ok")

	loop, _ := testutil.NewTestLoop(t,
		testutil.ToolCallResponse("Tick", "tc1", `{}`),
		testutil.ToolCallResponse("Tick", "tc2", `{}`),
		testutil.EndTurnResponseWithUsage("done", 20, 10),
	)
	loop.AddUser("go", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Turns != 3 {
		t.Errorf("expected 3 turns, got %d", result.Turns)
	}

	// Each of the first 2 responses has 10+5 usage, third has 20+10
	// Total: 10+10+20=40 input, 5+5+10=20 output
	if result.Tokens.InputTokens != 40 {
		t.Errorf("expected 40 input tokens, got %d", result.Tokens.InputTokens)
	}
	if result.Tokens.OutputTokens != 20 {
		t.Errorf("expected 20 output tokens, got %d", result.Tokens.OutputTokens)
	}
	if result.Tokens.TotalTokens != 60 {
		t.Errorf("expected 60 total tokens, got %d", result.Tokens.TotalTokens)
	}
}

func TestLoop_StreamChunks(t *testing.T) {
	loop, _ := testutil.NewTestLoop(t,
		testutil.EndTurnResponse("streamed response"),
	)
	loop.AddUser("hello", nil)

	ch := loop.Stream(context.Background())
	resp, err := core.Collect(context.Background(), ch)
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	if resp.Content != "streamed response" {
		t.Errorf("expected 'streamed response', got %q", resp.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", resp.StopReason)
	}
}

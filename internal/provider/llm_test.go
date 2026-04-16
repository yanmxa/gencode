package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
)

// --- mock provider for LLM tests ---

type mockLLMProvider struct {
	responses []core.CompletionResponse
	callIdx   int
	models    []ModelInfo
	listErr   error
	lastOpts  CompletionOptions
}

func (m *mockLLMProvider) Stream(_ context.Context, opts CompletionOptions) <-chan core.StreamChunk {
	m.lastOpts = opts
	ch := make(chan core.StreamChunk, 1)
	go func() {
		defer close(ch)
		if m.callIdx >= len(m.responses) {
			ch <- core.StreamChunk{Type: core.ChunkTypeDone, Response: &core.CompletionResponse{
				Content:    "no more responses",
				StopReason: "end_turn",
			}}
			return
		}
		resp := m.responses[m.callIdx]
		m.callIdx++
		ch <- core.StreamChunk{Type: core.ChunkTypeDone, Response: &resp}
	}()
	return ch
}

func (m *mockLLMProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return m.models, m.listErr
}

func (m *mockLLMProvider) Name() string { return "mock" }

// --- LLM tests ---

func TestLLMSend(t *testing.T) {
	mp := &mockLLMProvider{
		responses: []core.CompletionResponse{
			{Content: "hello", StopReason: "end_turn", Usage: core.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	l := &Client{provider: mp, model: "test-model", maxTokens: 4096}

	msgs := []core.Message{{Role: core.RoleUser, Content: "hi"}}
	resp, err := l.send(context.Background(), msgs, nil, "system prompt")
	if err != nil {
		t.Fatalf("send() error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got '%s'", resp.Content)
	}
}

func TestLLMStream(t *testing.T) {
	mp := &mockLLMProvider{
		responses: []core.CompletionResponse{
			{Content: "streamed", StopReason: "end_turn"},
		},
	}
	l := &Client{provider: mp, model: "test-model"}

	msgs := []core.Message{{Role: core.RoleUser, Content: "hi"}}
	ch := l.Stream(context.Background(), msgs, nil, "")

	var resp *core.CompletionResponse
	for chunk := range ch {
		if chunk.Type == core.ChunkTypeDone {
			resp = chunk.Response
		}
	}
	if resp == nil {
		t.Fatal("expected response from stream")
	}
	if resp.Content != "streamed" {
		t.Errorf("expected 'streamed', got '%s'", resp.Content)
	}
}

func TestLLMComplete(t *testing.T) {
	mp := &mockLLMProvider{
		responses: []core.CompletionResponse{
			{Content: "summary", StopReason: "end_turn"},
		},
	}
	l := &Client{provider: mp, model: "test-model"}

	resp, err := l.Complete(context.Background(), "compact", []core.Message{{Role: core.RoleUser, Content: "summarize"}}, 2048)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "summary" {
		t.Errorf("expected 'summary', got '%s'", resp.Content)
	}
}

func TestLLMNameAndModelID(t *testing.T) {
	l := &Client{provider: &mockLLMProvider{}, model: "claude-3"}
	if l.Name() != "mock" {
		t.Errorf("expected 'mock', got '%s'", l.Name())
	}
	if l.ModelID() != "claude-3" {
		t.Errorf("expected 'claude-3', got '%s'", l.ModelID())
	}
}

func TestResolveMaxTokens_CustomOverride(t *testing.T) {
	l := &Client{provider: &mockLLMProvider{}, model: "m", maxTokens: 16384}
	got := l.ResolveMaxTokens(context.Background())
	if got != 16384 {
		t.Errorf("expected 16384, got %d", got)
	}
}

func TestResolveMaxTokens_FromProvider(t *testing.T) {
	mp := &mockLLMProvider{
		models: []ModelInfo{
			{ID: "claude-opus", OutputTokenLimit: 32000},
			{ID: "claude-sonnet", OutputTokenLimit: 64000},
		},
	}
	l := &Client{provider: mp, model: "claude-sonnet"} // maxTokens = 0

	got := l.ResolveMaxTokens(context.Background())
	if got != 64000 {
		t.Errorf("expected 64000, got %d", got)
	}
}

func TestResolveMaxTokens_Fallback(t *testing.T) {
	mp := &mockLLMProvider{
		models: []ModelInfo{
			{ID: "other-model", OutputTokenLimit: 32000},
		},
	}
	l := &Client{provider: mp, model: "unknown-model"} // no match

	got := l.ResolveMaxTokens(context.Background())
	if got != defaultMaxTokens {
		t.Errorf("expected default %d, got %d", defaultMaxTokens, got)
	}
}

func TestCompletionOptsDefaultMaxTokens(t *testing.T) {
	l := &Client{provider: &mockLLMProvider{}, model: "m"}
	opts := l.completionOpts(nil, nil, "")
	if opts.MaxTokens != defaultMaxTokens {
		t.Errorf("expected default %d, got %d", defaultMaxTokens, opts.MaxTokens)
	}
}

func TestCompletionOptsIncludesThinkingLevel(t *testing.T) {
	l := &Client{
		provider:      &mockLLMProvider{},
		model:         "m",
		thinkingLevel: ThinkingHigh,
	}
	opts := l.completionOpts(nil, nil, "system")
	if opts.ThinkingLevel != ThinkingHigh {
		t.Fatalf("expected thinking level %v, got %v", ThinkingHigh, opts.ThinkingLevel)
	}
	if opts.SystemPrompt != "system" {
		t.Fatalf("expected system prompt to be preserved, got %q", opts.SystemPrompt)
	}
}

func TestOutputLimitFromProviderNil(t *testing.T) {
	got := outputLimitFromProvider(nil, "m")
	if got != 0 {
		t.Fatalf("expected nil provider to return 0, got %d", got)
	}
}

func TestOutputLimitFromProviderListModelsError(t *testing.T) {
	got := outputLimitFromProvider(&mockLLMProvider{listErr: errors.New("boom")}, "m")
	if got != 0 {
		t.Fatalf("expected ListModels error to return 0, got %d", got)
	}
}

// --- FakeLLM tests ---

func TestFakeLLMSend(t *testing.T) {
	fake := &FakeLLM{
		Responses: []core.CompletionResponse{
			{Content: "response 1", StopReason: "end_turn"},
			{Content: "response 2", StopReason: "end_turn"},
		},
	}

	resp1, err := fake.Send(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if resp1.Content != "response 1" {
		t.Errorf("expected 'response 1', got '%s'", resp1.Content)
	}

	resp2, err := fake.Send(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if resp2.Content != "response 2" {
		t.Errorf("expected 'response 2', got '%s'", resp2.Content)
	}

	// Exhausted — should return default
	resp3, _ := fake.Send(context.Background(), nil, nil, "")
	if resp3.Content != "no more responses" {
		t.Errorf("expected 'no more responses', got '%s'", resp3.Content)
	}
}

func TestFakeLLMStream(t *testing.T) {
	fake := &FakeLLM{
		Responses: []core.CompletionResponse{
			{Content: "streamed", StopReason: "end_turn", Usage: core.Usage{InputTokens: 5, OutputTokens: 3}},
		},
	}

	ch := fake.Stream(context.Background(), nil, nil, "")
	var resp *core.CompletionResponse
	for chunk := range ch {
		if chunk.Type == core.ChunkTypeDone {
			resp = chunk.Response
		}
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Content != "streamed" {
		t.Errorf("expected 'streamed', got '%s'", resp.Content)
	}
	if resp.Usage.InputTokens != 5 {
		t.Errorf("expected 5 input tokens, got %d", resp.Usage.InputTokens)
	}
}

func TestFakeLLMWithToolCalls(t *testing.T) {
	fake := &FakeLLM{
		Responses: []core.CompletionResponse{
			{
				Content:    "",
				StopReason: "tool_use",
				ToolCalls: []core.ToolCall{
					{ID: "tc1", Name: "Read", Input: `{"file_path": "/tmp/test"}`},
				},
			},
			{Content: "done", StopReason: "end_turn"},
		},
	}

	// First call returns tool calls
	resp1, _ := fake.Send(context.Background(), nil, nil, "")
	if len(resp1.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp1.ToolCalls))
	}
	if resp1.ToolCalls[0].Name != "Read" {
		t.Errorf("expected tool 'Read', got '%s'", resp1.ToolCalls[0].Name)
	}

	// Second call returns final response
	resp2, _ := fake.Send(context.Background(), nil, nil, "")
	if resp2.Content != "done" {
		t.Errorf("expected 'done', got '%s'", resp2.Content)
	}
}

func TestFakeLLMComplete(t *testing.T) {
	fake := &FakeLLM{
		Responses: []core.CompletionResponse{
			{Content: "summary", StopReason: "end_turn"},
		},
	}

	resp, err := fake.Complete(context.Background(), "compact", nil, 2048)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "summary" {
		t.Errorf("expected 'summary', got '%s'", resp.Content)
	}
}

func TestFakeLLMRecordsCalls(t *testing.T) {
	fake := &FakeLLM{
		Responses: []core.CompletionResponse{
			{Content: "ok", StopReason: "end_turn"},
		},
	}

	msgs := []core.Message{{Role: core.RoleUser, Content: "hello"}}
	tools := []ToolSchema{{Name: "Read", Description: "read files"}}
	fake.Send(context.Background(), msgs, tools, "sys prompt")

	if len(fake.Calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(fake.Calls))
	}
	call := fake.Calls[0]
	if call.SystemPrompt != "sys prompt" {
		t.Errorf("expected system prompt 'sys prompt', got '%s'", call.SystemPrompt)
	}
	if len(call.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(call.Messages))
	}
	if len(call.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(call.Tools))
	}
}

func TestFakeLLMDefaults(t *testing.T) {
	fake := &FakeLLM{}
	if fake.Name() != "fake" {
		t.Errorf("expected 'fake', got '%s'", fake.Name())
	}
	if fake.ModelID() != "fake-model" {
		t.Errorf("expected 'fake-model', got '%s'", fake.ModelID())
	}
	if fake.ResolveMaxTokens(context.Background()) != defaultMaxTokens {
		t.Errorf("expected %d, got %d", defaultMaxTokens, fake.ResolveMaxTokens(context.Background()))
	}
}

func TestFakeLLMCustomNames(t *testing.T) {
	fake := &FakeLLM{
		Model:        "gpt-4",
		ProviderName: "openai",
	}
	if fake.Name() != "openai" {
		t.Errorf("expected 'openai', got '%s'", fake.Name())
	}
	if fake.ModelID() != "gpt-4" {
		t.Errorf("expected 'gpt-4', got '%s'", fake.ModelID())
	}
}

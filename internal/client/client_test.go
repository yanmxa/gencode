package client

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// --- mock provider for Client tests ---

type mockProvider struct {
	responses []message.CompletionResponse
	callIdx   int
	models    []provider.ModelInfo
}

func (m *mockProvider) Stream(_ context.Context, opts provider.CompletionOptions) <-chan message.StreamChunk {
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

func (m *mockProvider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return m.models, nil
}

func (m *mockProvider) Name() string { return "mock" }

// --- Client tests ---

func TestClientSend(t *testing.T) {
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "hello", StopReason: "end_turn", Usage: message.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	c := &Client{Provider: mp, Model: "test-model", MaxTokens: 4096}

	msgs := []message.Message{{Role: message.RoleUser, Content: "hi"}}
	resp, err := c.Send(context.Background(), msgs, nil, "system prompt")
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got '%s'", resp.Content)
	}
}

func TestClientStream(t *testing.T) {
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "streamed", StopReason: "end_turn"},
		},
	}
	c := &Client{Provider: mp, Model: "test-model"}

	msgs := []message.Message{{Role: message.RoleUser, Content: "hi"}}
	ch := c.Stream(context.Background(), msgs, nil, "")

	var resp *message.CompletionResponse
	for chunk := range ch {
		if chunk.Type == message.ChunkTypeDone {
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

func TestClientComplete(t *testing.T) {
	mp := &mockProvider{
		responses: []message.CompletionResponse{
			{Content: "summary", StopReason: "end_turn"},
		},
	}
	c := &Client{Provider: mp, Model: "test-model"}

	resp, err := c.Complete(context.Background(), "compact", []message.Message{{Role: message.RoleUser, Content: "summarize"}}, 2048)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "summary" {
		t.Errorf("expected 'summary', got '%s'", resp.Content)
	}
}

func TestClientNameAndModelID(t *testing.T) {
	c := &Client{Provider: &mockProvider{}, Model: "claude-3"}
	if c.Name() != "mock" {
		t.Errorf("expected 'mock', got '%s'", c.Name())
	}
	if c.ModelID() != "claude-3" {
		t.Errorf("expected 'claude-3', got '%s'", c.ModelID())
	}
}

func TestResolveMaxTokens_CustomOverride(t *testing.T) {
	c := &Client{Provider: &mockProvider{}, Model: "m", MaxTokens: 16384}
	got := c.ResolveMaxTokens(context.Background())
	if got != 16384 {
		t.Errorf("expected 16384, got %d", got)
	}
}

func TestResolveMaxTokens_FromProvider(t *testing.T) {
	mp := &mockProvider{
		models: []provider.ModelInfo{
			{ID: "claude-opus", OutputTokenLimit: 32000},
			{ID: "claude-sonnet", OutputTokenLimit: 64000},
		},
	}
	c := &Client{Provider: mp, Model: "claude-sonnet"} // MaxTokens = 0

	got := c.ResolveMaxTokens(context.Background())
	if got != 64000 {
		t.Errorf("expected 64000, got %d", got)
	}
}

func TestResolveMaxTokens_Fallback(t *testing.T) {
	mp := &mockProvider{
		models: []provider.ModelInfo{
			{ID: "other-model", OutputTokenLimit: 32000},
		},
	}
	c := &Client{Provider: mp, Model: "unknown-model"} // no match

	got := c.ResolveMaxTokens(context.Background())
	if got != defaultMaxTokens {
		t.Errorf("expected default %d, got %d", defaultMaxTokens, got)
	}
}

func TestOptsDefaultMaxTokens(t *testing.T) {
	c := &Client{Provider: &mockProvider{}, Model: "m"}
	opts := c.opts(nil, nil, "")
	if opts.MaxTokens != defaultMaxTokens {
		t.Errorf("expected default %d, got %d", defaultMaxTokens, opts.MaxTokens)
	}
}

// --- FakeClient tests ---

func TestFakeClientSend(t *testing.T) {
	fake := &FakeClient{
		Responses: []message.CompletionResponse{
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

func TestFakeClientStream(t *testing.T) {
	fake := &FakeClient{
		Responses: []message.CompletionResponse{
			{Content: "streamed", StopReason: "end_turn", Usage: message.Usage{InputTokens: 5, OutputTokens: 3}},
		},
	}

	ch := fake.Stream(context.Background(), nil, nil, "")
	var resp *message.CompletionResponse
	for chunk := range ch {
		if chunk.Type == message.ChunkTypeDone {
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

func TestFakeClientWithToolCalls(t *testing.T) {
	fake := &FakeClient{
		Responses: []message.CompletionResponse{
			{
				Content:    "",
				StopReason: "tool_use",
				ToolCalls: []message.ToolCall{
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

func TestFakeClientComplete(t *testing.T) {
	fake := &FakeClient{
		Responses: []message.CompletionResponse{
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

func TestFakeClientRecordsCalls(t *testing.T) {
	fake := &FakeClient{
		Responses: []message.CompletionResponse{
			{Content: "ok", StopReason: "end_turn"},
		},
	}

	msgs := []message.Message{{Role: message.RoleUser, Content: "hello"}}
	tools := []provider.Tool{{Name: "Read", Description: "read files"}}
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

func TestFakeClientDefaults(t *testing.T) {
	fake := &FakeClient{}
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

func TestFakeClientCustomNames(t *testing.T) {
	fake := &FakeClient{
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

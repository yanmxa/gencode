package agent_test

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

func TestAgent_ExploreAgent(t *testing.T) {
	mp := &testutil.MockProvider{
		Responses: []message.CompletionResponse{
			{Content: "Explored the codebase", StopReason: "end_turn",
				Usage: message.Usage{InputTokens: 50, OutputTokens: 25}},
		},
	}

	executor := agent.NewExecutor(mp, t.TempDir(), "fake-model", nil)
	result, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:       "Explore",
		Prompt:      "Find all Go files",
		Description: "explore codebase",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.AgentName != "Explore" {
		t.Errorf("expected agent name 'Explore', got %q", result.AgentName)
	}
	if result.Content != "Explored the codebase" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestAgent_UnknownAgent(t *testing.T) {
	mp := &testutil.MockProvider{}
	executor := agent.NewExecutor(mp, t.TempDir(), "fake-model", nil)

	_, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:  "NonExistent",
		Prompt: "do something",
	})
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestAgent_MaxTurnsRespected(t *testing.T) {
	// LLM always returns tool calls to force hitting max turns
	responses := make([]message.CompletionResponse, 10)
	for i := range responses {
		responses[i] = message.CompletionResponse{
			StopReason: "tool_use",
			ToolCalls:  []message.ToolCall{{ID: "tc", Name: "UnknownTool", Input: "{}"}},
			Usage:      message.Usage{InputTokens: 1, OutputTokens: 1},
		}
	}

	executor := agent.NewExecutor(
		&testutil.MockProvider{Responses: responses},
		t.TempDir(), "fake-model", nil,
	)
	result, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:    "Explore",
		Prompt:   "keep going",
		MaxTurns: 2,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Success {
		t.Error("expected failure (max turns)")
	}
	if result.Error == "" {
		t.Error("expected error message about max turns")
	}
}

func TestAgent_ModelResolution(t *testing.T) {
	tests := []struct {
		name        string
		reqModel    string
		parentModel string
	}{
		{"request override", "custom-model", "parent-model"},
		{"parent inherited", "", "parent-model"},
		{"fallback", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := &testutil.MockProvider{
				Responses: []message.CompletionResponse{
					{Content: "ok", StopReason: "end_turn"},
				},
			}
			executor := agent.NewExecutor(mp, t.TempDir(), tt.parentModel, nil)

			if tt.parentModel != "" && executor.GetParentModelID() != tt.parentModel {
				t.Errorf("parent model mismatch: got %q, want %q",
					executor.GetParentModelID(), tt.parentModel)
			}

			_, err := executor.Run(context.Background(), agent.AgentRequest{
				Agent:  "Explore",
				Prompt: "test",
				Model:  tt.reqModel,
			})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
		})
	}
}

func TestAgent_BackgroundExecution(t *testing.T) {
	mp := &testutil.MockProvider{
		Responses: []message.CompletionResponse{
			{Content: "background result", StopReason: "end_turn",
				Usage: message.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}

	executor := agent.NewExecutor(mp, t.TempDir(), "fake-model", nil)
	agentTask, err := executor.RunBackground(agent.AgentRequest{
		Agent:       "Explore",
		Prompt:      "background task",
		Description: "bg test",
	})
	if err != nil {
		t.Fatalf("RunBackground() error: %v", err)
	}
	if agentTask == nil {
		t.Fatal("expected non-nil agent task")
	}

	// Wait for completion
	<-agentTask.GetContext().Done()

	info := agentTask.GetStatus()
	if info.Type != "agent" {
		t.Errorf("expected type 'agent', got %q", string(info.Type))
	}
}

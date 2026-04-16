package subagent_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

func TestAgent_ExploreAgent(t *testing.T) {
	mp := &testutil.MockProvider{
		Responses: []core.CompletionResponse{
			{
				Content: "Explored the codebase", StopReason: "end_turn",
				Usage: core.Usage{InputTokens: 50, OutputTokens: 25},
			},
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
	responses := make([]core.CompletionResponse, 10)
	for i := range responses {
		responses[i] = core.CompletionResponse{
			StopReason: "tool_use",
			ToolCalls:  []core.ToolCall{{ID: "tc", Name: "UnknownTool", Input: "{}"}},
			Usage:      core.Usage{InputTokens: 1, OutputTokens: 1},
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
				Responses: []core.CompletionResponse{
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

// TestAgent_PlanPermissionMode_BlocksWrites verifies that an agent configured
// with permission-mode:plan cannot execute write tools.
// The built-in "Explore" agent has PermissionPlan, so its permission checker
// should reject Write/Edit calls.
func TestAgent_PlanPermissionMode_BlocksWrites(t *testing.T) {
	// Verify via the permission package directly: PermissionPlan maps to ReadOnly()
	// which rejects any non-read-only tool.
	checker := permission.ReadOnly()

	writeTools := []string{"Write", "Edit", "NotebookEdit", "Bash"}
	for _, tool := range writeTools {
		decision := checker.Check(tool, nil)
		if decision != permission.Reject {
			t.Errorf("tool %q: expected Reject in plan mode, got %v", tool, decision)
		}
	}

	readTools := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"}
	for _, tool := range readTools {
		decision := checker.Check(tool, nil)
		if decision != permission.Permit {
			t.Errorf("tool %q: expected Permit in plan mode, got %v", tool, decision)
		}
	}

	// Also verify at the executor level: run a plan-mode agent with a Write tool
	// call queued. The tool call should be rejected (not executed), and the agent
	// should still complete because the LLM gets the error result and ends turn.
	mp := &testutil.MockProvider{
		Responses: []core.CompletionResponse{
			// First response: LLM tries to write a file
			{
				StopReason: "tool_use",
				ToolCalls: []core.ToolCall{
					{ID: "tc1", Name: "Write", Input: `{"file_path":"/tmp/x.txt","content":"hello"}`},
				},
				Usage: core.Usage{InputTokens: 20, OutputTokens: 10},
			},
			// Second response: LLM acknowledges the error and ends
			{
				Content:    "Cannot write files in plan mode",
				StopReason: "end_turn",
				Usage:      core.Usage{InputTokens: 30, OutputTokens: 15},
			},
		},
	}

	executor := agent.NewExecutor(mp, t.TempDir(), "fake-model", nil)
	result, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:  "Explore", // built-in, PermissionPlan
		Prompt: "try to write a file",
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success (agent ended turn after rejection), got error: %s", result.Error)
	}

	// The result content should come from the second response
	if !strings.Contains(result.Content, "Cannot write files in plan mode") {
		t.Errorf("unexpected final content: %q", result.Content)
	}
}

// TestAgent_SubagentHooks_Fire verifies that SubagentStart and SubagentStop
// hooks are fired when an agent runs.
func TestAgent_SubagentHooks_Fire(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sentinel files that our hook scripts will touch
	startFile := filepath.Join(tmpDir, "subagent_start.txt")
	stopFile := filepath.Join(tmpDir, "subagent_stop.txt")

	// Build a settings object with SubagentStart and SubagentStop hooks.
	// Each hook writes to a temp file so we can verify it fired.
	settings := &config.Settings{
		Hooks: map[string][]config.Hook{
			string(hooks.SubagentStart): {
				{
					Matcher: "",
					Hooks: []config.HookCmd{
						{Type: "command", Command: "touch " + startFile, Async: false},
					},
				},
			},
			string(hooks.SubagentStop): {
				{
					Matcher: "",
					Hooks: []config.HookCmd{
						{Type: "command", Command: "touch " + stopFile, Async: false},
					},
				},
			},
		},
	}

	engine := hooks.NewEngine(settings, "test-session-id", tmpDir, "")

	mp := &testutil.MockProvider{
		Responses: []core.CompletionResponse{
			{
				Content:    "done",
				StopReason: "end_turn",
				Usage:      core.Usage{InputTokens: 10, OutputTokens: 5},
			},
		},
	}

	executor := agent.NewExecutor(mp, tmpDir, "fake-model", engine)
	_, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:  "Explore",
		Prompt: "test hooks",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Both SubagentStart and SubagentStop are fired via ExecuteAsync (goroutines),
	// so poll briefly for each sentinel file to appear.
	waitForFile := func(path, label string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(path); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("%s hook did not fire within 3s (sentinel file missing: %s)", label, path)
	}

	waitForFile(startFile, "SubagentStart")
	waitForFile(stopFile, "SubagentStop")
}

func TestAgent_BackgroundExecution(t *testing.T) {
	mp := &testutil.MockProvider{
		Responses: []core.CompletionResponse{
			{
				Content: "background result", StopReason: "end_turn",
				Usage: core.Usage{InputTokens: 10, OutputTokens: 5},
			},
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

func TestAgent_OnProgressReceivesToolUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	readme := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readme, []byte("hello from agent"), 0o644); err != nil {
		t.Fatalf("WriteFile(README): %v", err)
	}

	mp := &testutil.MockProvider{
		Responses: []core.CompletionResponse{
			{
				StopReason: "tool_use",
				ToolCalls: []core.ToolCall{
					{
						ID:    "tc1",
						Name:  "Read",
						Input: `{"file_path":"README.md"}`,
					},
				},
			},
			{
				Content:    "Read complete",
				StopReason: "end_turn",
			},
		},
	}

	executor := agent.NewExecutor(mp, tmpDir, "fake-model", nil)
	var progress []string
	result, err := executor.Run(context.Background(), agent.AgentRequest{
		Agent:  "Explore",
		Prompt: "inspect the readme",
		OnProgress: func(msg string) {
			progress = append(progress, msg)
		},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if len(progress) != 1 || progress[0] != "Read(README.md)" {
		t.Fatalf("unexpected progress callback values: %#v", progress)
	}
	if len(result.Progress) != 1 || result.Progress[0] != "Read(README.md)" {
		t.Fatalf("unexpected result progress values: %#v", result.Progress)
	}
}

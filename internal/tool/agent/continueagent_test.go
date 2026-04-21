package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
)

type stubContinueAgentExecutor struct {
	lastRun           tool.AgentExecRequest
	lastBackgroundRun tool.AgentExecRequest
	runResult         *tool.AgentExecResult
	backgroundResult  tool.AgentTaskInfo
}

func (s *stubContinueAgentExecutor) Run(ctx context.Context, req tool.AgentExecRequest) (*tool.AgentExecResult, error) {
	s.lastRun = req
	if s.runResult == nil {
		s.runResult = &tool.AgentExecResult{
			AgentID:     "agent-resumed-2",
			AgentName:   "Explore",
			Model:       "sonnet",
			Success:     true,
			Content:     "done",
			TurnCount:   2,
			ToolUses:    1,
			TotalTokens: 42,
		}
	}
	return s.runResult, nil
}

func (s *stubContinueAgentExecutor) RunBackground(req tool.AgentExecRequest) (tool.AgentTaskInfo, error) {
	s.lastBackgroundRun = req
	if s.backgroundResult.TaskID == "" {
		s.backgroundResult = tool.AgentTaskInfo{TaskID: "bg-continued-1", AgentName: "Explore"}
	}
	return s.backgroundResult, nil
}

func (s *stubContinueAgentExecutor) GetAgentConfig(agentType string) (tool.AgentConfigInfo, bool) {
	return tool.AgentConfigInfo{
		Name:           agentType,
		Description:    "test agent",
		PermissionMode: "default",
		Tools:          []string{"Read"},
	}, true
}

func (s *stubContinueAgentExecutor) GetParentModelID() string {
	return "sonnet"
}

func TestContinueAgentTool_ResolvesTaskIDToResumeID(t *testing.T) {
	task.Initialize(task.Options{})
	t.Cleanup(task.ResetService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("task-agent-continue", "Explore Worker", "Initial task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-123")
	agentTask.Complete(nil)
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("task-agent-continue")

	executor := &stubContinueAgentExecutor{}
	toolInst := NewContinueAgentTool()
	toolInst.SetExecutor(executor)

	result := toolInst.Execute(context.Background(), map[string]any{
		"task_id":     "task-agent-continue",
		"prompt":      "Keep going",
		"description": "Continue audit",
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if executor.lastRun.ResumeID != "agent-session-123" {
		t.Fatalf("ResumeID = %q, want %q", executor.lastRun.ResumeID, "agent-session-123")
	}
	if executor.lastRun.Agent != "Explore" {
		t.Fatalf("Agent = %q, want %q", executor.lastRun.Agent, "Explore")
	}
	if executor.lastRun.Name != "Explore Worker" {
		t.Fatalf("Name = %q, want %q", executor.lastRun.Name, "Explore Worker")
	}
}

func TestContinueAgentTool_BackgroundContinuation(t *testing.T) {
	executor := &stubContinueAgentExecutor{}
	toolInst := NewContinueAgentTool()
	toolInst.SetExecutor(executor)

	result := toolInst.Execute(context.Background(), map[string]any{
		"agent_id":          "agent-session-777",
		"subagent_type":     "Explore",
		"prompt":            "Retry with narrower scope",
		"description":       "Retry audit",
		"run_in_background": true,
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if executor.lastBackgroundRun.ResumeID != "agent-session-777" {
		t.Fatalf("ResumeID = %q, want %q", executor.lastBackgroundRun.ResumeID, "agent-session-777")
	}
	if !executor.lastBackgroundRun.Background {
		t.Fatal("expected background continuation request")
	}
	if !strings.Contains(result.Output, "Agent continuation started in background.") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}

func TestContinueAgentTool_RequiresAgentTypeForDirectAgentID(t *testing.T) {
	toolInst := NewContinueAgentTool()
	toolInst.SetExecutor(&stubContinueAgentExecutor{})

	result := toolInst.Execute(context.Background(), map[string]any{
		"agent_id": "agent-session-999",
		"prompt":   "Continue",
	}, ".")

	if result.Success {
		t.Fatal("expected failure")
	}
	if !strings.Contains(result.Error, "subagent_type is required") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

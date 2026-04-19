package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
)

func TestSendMessageTool_ResumesCompletedTask(t *testing.T) {
	task.Initialize(task.Options{})
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(task.ResetService)
	t.Cleanup(orchestration.ResetService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("task-sendmessage-1", "Explore Worker", "Initial task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-321")
	agentTask.Complete(nil)
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("task-sendmessage-1")

	executor := &stubContinueAgentExecutor{}
	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(executor)

	result := toolInst.Execute(context.Background(), map[string]any{
		"task_id":     "task-sendmessage-1",
		"message":     "Please verify the earlier finding",
		"description": "Verify finding",
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if executor.lastRun.ResumeID != "agent-session-321" {
		t.Fatalf("ResumeID = %q, want %q", executor.lastRun.ResumeID, "agent-session-321")
	}
	if executor.lastRun.Prompt != "Please verify the earlier finding" {
		t.Fatalf("Prompt = %q", executor.lastRun.Prompt)
	}
}

func TestSendMessageTool_RejectsRunningTask(t *testing.T) {
	task.Initialize(task.Options{})
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(task.ResetService)
	t.Cleanup(orchestration.ResetService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("task-sendmessage-2", "Explore Worker", "Initial task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-654")
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("task-sendmessage-2")
	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:    "task-sendmessage-2",
		AgentID:   "agent-session-654",
		AgentType: "Explore",
		Running:   true,
		Status:    string(task.StatusRunning),
	})

	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(&stubContinueAgentExecutor{})

	result := toolInst.Execute(context.Background(), map[string]any{
		"task_id": "task-sendmessage-2",
		"message": "Keep going",
	}, ".")

	if !result.Success {
		t.Fatalf("expected queued success for running task, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "queued for delivery at the worker's next safe turn boundary") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
	if strings.Contains(result.Output, "Use TaskOutput") {
		t.Fatalf("should not encourage immediate TaskOutput polling: %s", result.Output)
	}
	if orchestration.Default().PendingMessageCount("task-sendmessage-2", "") != 1 {
		t.Fatalf("expected queued message count to be 1")
	}
}

func TestSendMessageTool_BackgroundMessageByAgentID(t *testing.T) {
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(orchestration.ResetService)

	executor := &stubContinueAgentExecutor{}
	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(executor)

	result := toolInst.Execute(context.Background(), map[string]any{
		"agent_id":          "agent-session-777",
		"subagent_type":     "Explore",
		"message":           "Retry with narrower scope",
		"description":       "Retry audit",
		"run_in_background": true,
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if executor.lastBackgroundRun.ResumeID != "agent-session-777" {
		t.Fatalf("ResumeID = %q, want %q", executor.lastBackgroundRun.ResumeID, "agent-session-777")
	}
	if executor.lastBackgroundRun.Prompt != "Retry with narrower scope" {
		t.Fatalf("Prompt = %q", executor.lastBackgroundRun.Prompt)
	}
	if !strings.Contains(result.Output, "Message sent to worker in background.") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
	if strings.Contains(result.Output, "Use TaskOutput") {
		t.Fatalf("should not encourage immediate TaskOutput polling: %s", result.Output)
	}
}

func TestSendMessageTool_RequiresAgentTypeForDirectAgentID(t *testing.T) {
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(orchestration.ResetService)

	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(&stubContinueAgentExecutor{})

	result := toolInst.Execute(context.Background(), map[string]any{
		"agent_id": "agent-session-999",
		"message":  "Continue",
	}, ".")

	if result.Success {
		t.Fatal("expected failure")
	}
	if !strings.Contains(result.Error, "subagent_type is required") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestSendMessageTool_DrainsQueuedMessagesOnResume(t *testing.T) {
	task.Initialize(task.Options{})
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(task.ResetService)
	t.Cleanup(orchestration.ResetService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("task-sendmessage-3", "Explore Worker", "Initial task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-888")
	agentTask.Complete(nil)
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("task-sendmessage-3")

	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:    "task-sendmessage-3",
		AgentID:   "agent-session-888",
		AgentType: "Explore",
		Running:   false,
		Status:    string(task.StatusCompleted),
	})
	orchestration.Default().QueuePendingMessage("task-sendmessage-3", "First queued update")
	orchestration.Default().QueuePendingMessage("task-sendmessage-3", "Second queued update")

	executor := &stubContinueAgentExecutor{}
	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(executor)

	result := toolInst.Execute(context.Background(), map[string]any{
		"task_id": "task-sendmessage-3",
		"message": "Latest instruction",
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(executor.lastRun.Prompt, "First queued update") {
		t.Fatalf("expected prompt to include first queued message, got: %q", executor.lastRun.Prompt)
	}
	if !strings.Contains(executor.lastRun.Prompt, "Latest instruction") {
		t.Fatalf("expected prompt to include latest instruction, got: %q", executor.lastRun.Prompt)
	}
	if got := orchestration.Default().PendingMessageCount("task-sendmessage-3", ""); got != 0 {
		t.Fatalf("expected pending messages to drain, got %d", got)
	}
}

func TestSendMessageTool_QueuesRunningTaskByAgentID(t *testing.T) {
	task.Initialize(task.Options{})
	orchestration.Initialize(orchestration.Options{})
	t.Cleanup(task.ResetService)
	t.Cleanup(orchestration.ResetService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("task-sendmessage-4", "Explore Worker", "Initial task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-running")
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("task-sendmessage-4")

	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:    "task-sendmessage-4",
		AgentID:   "agent-session-running",
		AgentType: "Explore",
		Running:   true,
		Status:    string(task.StatusRunning),
	})

	toolInst := NewSendMessageTool()
	toolInst.SetExecutor(&stubContinueAgentExecutor{})

	result := toolInst.Execute(context.Background(), map[string]any{
		"agent_id":      "agent-session-running",
		"subagent_type": "Explore",
		"message":       "Narrow the scope to only import graphs",
	}, ".")

	if !result.Success {
		t.Fatalf("expected queued success, got: %s", result.Error)
	}
	if got := orchestration.Default().PendingMessageCount("task-sendmessage-4", ""); got != 1 {
		t.Fatalf("expected queued message count to be 1, got %d", got)
	}
}

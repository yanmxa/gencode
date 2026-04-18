package task

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
)

func TestTaskOutputTool_StillRunning(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	// Create a test agent task that stays running
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-123", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(5, 1000)
	agentTask.AppendOutput([]byte("Some partial output\n"))

	// Register the task
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-123")

	// Execute TaskOutput with short timeout
	tool := &TaskOutputTool{}
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-123",
		"block":   true,
		"timeout": float64(100), // 100ms timeout
	}, ".")

	// Verify it returns success (not error) for still-running task
	if !result.Success {
		t.Errorf("Expected Success=true for still-running task, got false. Error: %s", result.Error)
	}

	// Verify the output contains helpful information
	if !strings.Contains(result.Output, "still running") {
		t.Errorf("Expected 'still running' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "Turns: 5") {
		t.Errorf("Expected 'Turns: 5' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "Options:") {
		t.Errorf("Expected 'Options:' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "TaskOutput") {
		t.Errorf("Expected TaskOutput suggestion in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "TaskStop") {
		t.Errorf("Expected TaskStop suggestion in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_Completed(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	// Create a test agent task and complete it
	ctx, cancel := context.WithCancel(context.Background())

	agentTask := task.NewAgentTask("test-agent-456", "Explore", "Test task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-456")
	agentTask.SetOutputFile("/tmp/transcripts/agent-session-456.jsonl")
	agentTask.UpdateProgress(10, 2000)
	agentTask.AppendOutput([]byte("Final output\n"))
	agentTask.Complete(nil)
	cancel()

	// Register the task
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-456")

	// Execute TaskOutput
	tool := &TaskOutputTool{}
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-456",
		"block":   true,
		"timeout": float64(1000),
	}, ".")

	// Verify success
	if !result.Success {
		t.Errorf("Expected Success=true for completed task, got false. Error: %s", result.Error)
	}

	// Verify the output contains completion info
	if !strings.Contains(result.Output, "completed") {
		t.Errorf("Expected 'completed' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "Turns: 10") {
		t.Errorf("Expected 'Turns: 10' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "AgentID: agent-session-456") {
		t.Errorf("Expected resumable AgentID in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "OutputFile: /tmp/transcripts/agent-session-456.jsonl") {
		t.Errorf("Expected output file path in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "ContinueAgent(task_id=\"test-agent-456\"") {
		t.Errorf("Expected ContinueAgent suggestion in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_NotFound(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	tool := &TaskOutputTool{}
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "nonexistent-task",
		"block":   false,
	}, ".")

	if result.Success {
		t.Error("Expected Success=false for nonexistent task")
	}

	if !strings.Contains(result.Error, "not found") {
		t.Errorf("Expected 'not found' in error, got: %s", result.Error)
	}
}

func TestTaskOutputTool_NonBlocking(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	// Create a running task
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-789", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(3, 500)

	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-789")

	// Execute with block=false
	tool := &TaskOutputTool{}
	start := time.Now()
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-789",
		"block":   false,
	}, ".")
	elapsed := time.Since(start)

	// Should return immediately (not wait)
	if elapsed > 500*time.Millisecond {
		t.Errorf("Non-blocking call took too long: %v", elapsed)
	}

	if !result.Success {
		t.Errorf("Expected Success=true, got false. Error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "running") {
		t.Errorf("Expected 'running' in output, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "TaskOutput deferred") {
		t.Errorf("Expected deferred polling hint in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_DefaultsToNonBlocking(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-default", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(2, 300)

	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-default")

	tool := &TaskOutputTool{}
	start := time.Now()
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-default",
	}, ".")
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Default TaskOutput call should be non-blocking, took %v", elapsed)
	}

	if !result.Success {
		t.Errorf("Expected Success=true, got false. Error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "running") {
		t.Errorf("Expected 'running' in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_AllowsOlderRunningTaskInspection(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-older", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(7, 900)
	agentTask.StartTime = time.Now().Add(-recentLaunchPollingCooldown - time.Second)

	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-older")

	tool := &TaskOutputTool{}
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-older",
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Background task is still running.") {
		t.Fatalf("expected normal running output for older task, got: %s", result.Output)
	}
}

func TestTaskOutputTool_RendersOrchestrationSnapshot(t *testing.T) {
	orchestration.Default().Reset()
	t.Cleanup(orchestration.Default().Reset)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-orch", "Explore", "Test task", ctx, cancel)
	agentTask.SetIdentity("Explore", "agent-session-orch")
	agentTask.Complete(nil)
	task.Default().RegisterTask(agentTask)
	defer task.Default().Remove("test-agent-orch")

	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:       "test-agent-orch",
		AgentID:      "agent-session-orch",
		AgentType:    "Explore",
		AgentName:    "Explore",
		Description:  "Test task",
		Status:       string(task.StatusCompleted),
		Running:      false,
		BatchID:      "batch-orch",
		BatchKey:     "batch-orch",
		BatchSubject: "2 background agents launched",
		BatchTotal:   2,
	})
	orchestration.Default().UpdateBatch(orchestration.Batch{
		ID:        "batch-orch",
		Key:       "batch-orch",
		Subject:   "2 background agents launched",
		Status:    "completed",
		Completed: 2,
		Total:     2,
		Failures:  1,
	})
	orchestration.Default().QueuePendingMessage("test-agent-orch", "follow up later")

	tool := &TaskOutputTool{}
	result := tool.Execute(context.Background(), map[string]any{
		"task_id": "test-agent-orch",
	}, ".")

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Orchestration:") {
		t.Fatalf("expected orchestration section, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "PendingMessages: 1") {
		t.Fatalf("expected pending message count, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "CoordinatorPhase: completed_batch_with_failures") {
		t.Fatalf("expected coordinator phase, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "SuggestedNextActions: continue_failed_worker, spawn_verifier, finalize_summary") {
		t.Fatalf("expected suggested actions, got: %s", result.Output)
	}
}

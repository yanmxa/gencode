package tool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/task"
)

func TestTaskOutputTool_StillRunning(t *testing.T) {
	// Create a test agent task that stays running
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-123", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(5, 1000)
	agentTask.AppendOutput([]byte("Some partial output\n"))

	// Register the task
	task.DefaultManager.RegisterTask(agentTask)
	defer task.DefaultManager.Remove("test-agent-123")

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
	// Create a test agent task and complete it
	ctx, cancel := context.WithCancel(context.Background())

	agentTask := task.NewAgentTask("test-agent-456", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(10, 2000)
	agentTask.AppendOutput([]byte("Final output\n"))
	agentTask.Complete(nil)
	cancel()

	// Register the task
	task.DefaultManager.RegisterTask(agentTask)
	defer task.DefaultManager.Remove("test-agent-456")

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
}

func TestTaskOutputTool_NotFound(t *testing.T) {
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
	// Create a running task
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentTask := task.NewAgentTask("test-agent-789", "Explore", "Test task", ctx, cancel)
	agentTask.UpdateProgress(3, 500)

	task.DefaultManager.RegisterTask(agentTask)
	defer task.DefaultManager.Remove("test-agent-789")

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
}

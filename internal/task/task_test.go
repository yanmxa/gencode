package task

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestTask_Complete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("test-id", "echo test", "Test task", cmd, ctx, cancel)

	// Complete the task
	task.Complete(0, nil)

	if task.Status != StatusCompleted {
		t.Errorf("expected status 'completed', got '%s'", task.Status)
	}
	if task.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", task.ExitCode)
	}
}

func TestTask_Failed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("fail-id", "exit 1", "Failing task", cmd, ctx, cancel)

	// Complete with non-zero exit code
	task.Complete(1, nil)

	if task.Status != StatusFailed {
		t.Errorf("expected status 'failed', got '%s'", task.Status)
	}
	if task.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", task.ExitCode)
	}
}

func TestTask_Kill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("kill-id", "sleep 100", "Long task", cmd, ctx, cancel)

	task.Kill()

	if task.Status != StatusKilled {
		t.Errorf("expected status 'killed', got '%s'", task.Status)
	}
}

func TestTask_AppendAndGetOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("output-id", "echo test", "Output task", cmd, ctx, cancel)

	task.AppendOutput([]byte("line 1\n"))
	task.AppendOutput([]byte("line 2\n"))

	output := task.GetOutput()
	expected := "line 1\nline 2\n"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestTask_IsRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("running-id", "echo test", "Running task", cmd, ctx, cancel)

	if !task.IsRunning() {
		t.Error("task should be running initially")
	}

	task.Complete(0, nil)

	if task.IsRunning() {
		t.Error("task should not be running after completion")
	}
}

func TestTask_WaitForCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("wait-id", "echo test", "Wait task", cmd, ctx, cancel)

	// Complete in background after short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		task.Complete(0, nil)
	}()

	completed := task.WaitForCompletion(time.Second)
	if !completed {
		t.Error("expected task to complete within timeout")
	}
}

func TestTask_WaitForCompletionTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("timeout-id", "sleep 100", "Timeout task", cmd, ctx, cancel)

	// Don't complete the task, let it timeout
	completed := task.WaitForCompletion(200 * time.Millisecond)
	if completed {
		t.Error("expected timeout, but task completed")
	}
}

func TestTask_GetStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("status-id", "echo test", "Status task", cmd, ctx, cancel)
	task.AppendOutput([]byte("output\n"))

	info := task.GetStatus()

	if info.ID != "status-id" {
		t.Errorf("expected ID 'status-id', got '%s'", info.ID)
	}
	if info.Command != "echo test" {
		t.Errorf("expected command 'echo test', got '%s'", info.Command)
	}
	if info.Status != StatusRunning {
		t.Errorf("expected status Running, got '%s'", info.Status)
	}
	if info.Output != "output\n" {
		t.Errorf("expected output 'output\\n', got '%s'", info.Output)
	}
}

func TestTask_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.Start()

	task := NewTask("concurrent-id", "echo test", "Concurrent task", cmd, ctx, cancel)

	var wg sync.WaitGroup

	// Multiple goroutines reading and writing
	for i := 0; i < 10; i++ {
		wg.Add(3)

		go func() {
			defer wg.Done()
			task.AppendOutput([]byte("data\n"))
		}()

		go func() {
			defer wg.Done()
			_ = task.GetOutput()
		}()

		go func() {
			defer wg.Done()
			_ = task.IsRunning()
		}()
	}

	wg.Wait()

	// Complete should not panic with concurrent access
	task.Complete(0, nil)
}

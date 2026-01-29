package task

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"time"
)

// TaskStatus represents the status of a background task
type TaskStatus string

const (
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusKilled    TaskStatus = "killed"
)

// Task represents a background task
type Task struct {
	ID          string             // Unique task ID
	Command     string             // The command being executed
	Description string             // Brief description
	Status      TaskStatus         // Current status
	PID         int                // Process ID
	StartTime   time.Time          // When the task started
	EndTime     time.Time          // When the task ended (if completed)
	ExitCode    int                // Exit code (if completed)
	Error       string             // Error message (if failed)
	Cmd         *exec.Cmd          // The running command
	Ctx         context.Context    // Task context
	Cancel      context.CancelFunc // Cancel function

	mu     sync.RWMutex // Protects output buffer
	output bytes.Buffer  // Collected stdout/stderr
}

// NewTask creates a new task
func NewTask(id, command, description string, cmd *exec.Cmd, ctx context.Context, cancel context.CancelFunc) *Task {
	return &Task{
		ID:          id,
		Command:     command,
		Description: description,
		Status:      StatusRunning,
		PID:         cmd.Process.Pid,
		StartTime:   time.Now(),
		Cmd:         cmd,
		Ctx:         ctx,
		Cancel:      cancel,
	}
}

// AppendOutput appends data to the output buffer
func (t *Task) AppendOutput(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output.Write(data)
}

// GetOutput returns the current output
func (t *Task) GetOutput() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.output.String()
}

// Complete marks the task as completed
func (t *Task) Complete(exitCode int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.EndTime = time.Now()
	t.ExitCode = exitCode

	if err != nil {
		t.Status = StatusFailed
		t.Error = err.Error()
	} else if exitCode != 0 {
		t.Status = StatusFailed
	} else {
		t.Status = StatusCompleted
	}
}

// Kill marks the task as killed
func (t *Task) Kill() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = StatusKilled
	t.EndTime = time.Now()
}

// IsRunning returns true if the task is still running
func (t *Task) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status == StatusRunning
}

// WaitForCompletion waits until the task completes or timeout
// Returns true if completed, false if timeout
func (t *Task) WaitForCompletion(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for {
		t.mu.RLock()
		status := t.Status
		t.mu.RUnlock()

		if status != StatusRunning {
			return true // completed
		}

		if time.Now().After(deadline) {
			return false // timeout
		}

		// Poll with small sleep
		time.Sleep(100 * time.Millisecond)
	}
}

// GetStatus returns the current task status info
func (t *Task) GetStatus() TaskInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TaskInfo{
		ID:          t.ID,
		Command:     t.Command,
		Description: t.Description,
		Status:      t.Status,
		PID:         t.PID,
		StartTime:   t.StartTime,
		EndTime:     t.EndTime,
		ExitCode:    t.ExitCode,
		Error:       t.Error,
		Output:      t.output.String(),
	}
}

// TaskInfo is a snapshot of task information
type TaskInfo struct {
	ID          string
	Command     string
	Description string
	Status      TaskStatus
	PID         int
	StartTime   time.Time
	EndTime     time.Time
	ExitCode    int
	Error       string
	Output      string
}

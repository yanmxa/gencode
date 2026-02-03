package task

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// BashTask represents a background bash command task
// It implements the BackgroundTask interface
type BashTask struct {
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

	mu     sync.RWMutex // Protects output buffer and status
	output bytes.Buffer // Collected stdout/stderr
}

// Verify BashTask implements BackgroundTask
var _ BackgroundTask = (*BashTask)(nil)

// NewBashTask creates a new bash task
func NewBashTask(id, command, description string, cmd *exec.Cmd, ctx context.Context, cancel context.CancelFunc) *BashTask {
	return &BashTask{
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

// GetID returns the unique task identifier
func (t *BashTask) GetID() string {
	return t.ID
}

// GetType returns the task type
func (t *BashTask) GetType() TaskType {
	return TaskTypeBash
}

// GetDescription returns the task description
func (t *BashTask) GetDescription() string {
	return t.Description
}

// AppendOutput appends data to the output buffer
func (t *BashTask) AppendOutput(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output.Write(data)
}

// GetOutput returns the current output
func (t *BashTask) GetOutput() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.output.String()
}

// Complete marks the task as completed
func (t *BashTask) Complete(exitCode int, err error) {
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

// MarkKilled marks the task as killed (internal use)
func (t *BashTask) MarkKilled() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = StatusKilled
	t.EndTime = time.Now()
}

// IsRunning returns true if the task is still running
func (t *BashTask) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status == StatusRunning
}

// WaitForCompletion waits until the task completes or timeout
// Returns true if completed, false if timeout
func (t *BashTask) WaitForCompletion(timeout time.Duration) bool {
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

// Stop gracefully stops the task (SIGTERM)
func (t *BashTask) Stop() error {
	// Cancel the context first
	if t.Cancel != nil {
		t.Cancel()
	}

	// Send SIGTERM to process group
	if t.PID > 0 {
		if err := syscall.Kill(-t.PID, syscall.SIGTERM); err != nil {
			// Ignore if process already exited
			if err != syscall.ESRCH {
				return err
			}
		}
	}

	return nil
}

// Kill forcefully terminates the task (SIGKILL)
func (t *BashTask) Kill() error {
	// Cancel the context
	if t.Cancel != nil {
		t.Cancel()
	}

	// Send SIGKILL to process group
	if t.PID > 0 {
		if err := syscall.Kill(-t.PID, syscall.SIGKILL); err != nil {
			// Ignore if process already exited
			if err != syscall.ESRCH {
				return err
			}
		}
	}

	t.MarkKilled()
	return nil
}

// GetStatus returns the current task status info
func (t *BashTask) GetStatus() TaskInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TaskInfo{
		ID:          t.ID,
		Type:        TaskTypeBash,
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

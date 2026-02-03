package task

import (
	"bytes"
	"context"
	"sync"
	"time"
)

// ProgressUpdate represents a progress update from the agent
type ProgressUpdate struct {
	Message string // Progress message (e.g., "Reading: file.go")
	Done    bool   // True if task is complete
}

// AgentTask represents a background agent task
// It implements the BackgroundTask interface
type AgentTask struct {
	ID          string     // Unique task ID
	AgentName   string     // Name of the agent type (Explore, Plan, etc.)
	Description string     // Brief description of the task
	Status      TaskStatus // Current status
	StartTime   time.Time  // When the task started
	EndTime     time.Time  // When the task ended (if completed)
	TurnCount   int        // Number of conversation turns
	TokenUsage  int        // Total tokens consumed
	Error       string     // Error message (if failed)

	ctx    context.Context    // Task context
	cancel context.CancelFunc // Cancel function

	mu          sync.RWMutex          // Protects mutable fields
	output      bytes.Buffer          // Collected output from the agent
	subscribers []chan ProgressUpdate // Output subscribers
}

// Verify AgentTask implements BackgroundTask
var _ BackgroundTask = (*AgentTask)(nil)

// NewAgentTask creates a new agent task
func NewAgentTask(id, agentName, description string, ctx context.Context, cancel context.CancelFunc) *AgentTask {
	return &AgentTask{
		ID:          id,
		AgentName:   agentName,
		Description: description,
		Status:      StatusRunning,
		StartTime:   time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// GetID returns the unique task identifier
func (t *AgentTask) GetID() string {
	return t.ID
}

// GetType returns the task type
func (t *AgentTask) GetType() TaskType {
	return TaskTypeAgent
}

// GetDescription returns the task description
func (t *AgentTask) GetDescription() string {
	return t.Description
}

// Subscribe returns a channel that receives progress updates
// The channel is closed when the task completes
func (t *AgentTask) Subscribe() <-chan ProgressUpdate {
	ch := make(chan ProgressUpdate, 100)
	t.mu.Lock()
	t.subscribers = append(t.subscribers, ch)
	t.mu.Unlock()
	return ch
}

// notifySubscribers sends a progress update to all subscribers (non-blocking)
func (t *AgentTask) notifySubscribers(msg string, done bool) {
	update := ProgressUpdate{Message: msg, Done: done}
	for _, ch := range t.subscribers {
		select {
		case ch <- update:
		default:
			// Non-blocking: skip if channel is full
		}
	}
}

// closeSubscribers closes all subscriber channels
func (t *AgentTask) closeSubscribers() {
	for _, ch := range t.subscribers {
		close(ch)
	}
	t.subscribers = nil
}

// AppendOutput appends data to the output buffer and notifies subscribers
func (t *AgentTask) AppendOutput(data []byte) {
	t.mu.Lock()
	t.output.Write(data)
	subs := t.subscribers
	t.mu.Unlock()

	// Notify outside of lock
	if len(subs) > 0 && len(data) > 0 {
		t.notifySubscribers(string(data), false)
	}
}

// AppendProgress appends a progress message and notifies subscribers
func (t *AgentTask) AppendProgress(msg string) {
	t.mu.Lock()
	subs := t.subscribers
	t.mu.Unlock()

	if len(subs) > 0 {
		t.notifySubscribers(msg, false)
	}
}

// GetOutput returns the current output
func (t *AgentTask) GetOutput() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.output.String()
}

// Complete marks the task as completed and notifies subscribers
func (t *AgentTask) Complete(err error) {
	t.mu.Lock()
	t.EndTime = time.Now()

	if err != nil {
		t.Status = StatusFailed
		t.Error = err.Error()
	} else {
		t.Status = StatusCompleted
	}
	subs := t.subscribers
	t.mu.Unlock()

	// Notify completion and close channels
	if len(subs) > 0 {
		t.notifySubscribers("", true)
		t.mu.Lock()
		t.closeSubscribers()
		t.mu.Unlock()
	}
}

// MarkKilled marks the task as killed (internal use)
func (t *AgentTask) MarkKilled() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = StatusKilled
	t.EndTime = time.Now()
}

// IsRunning returns true if the task is still running
func (t *AgentTask) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status == StatusRunning
}

// WaitForCompletion waits until the task completes or timeout
// Returns true if completed, false if timeout
func (t *AgentTask) WaitForCompletion(timeout time.Duration) bool {
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

// Stop gracefully stops the task by canceling the context
func (t *AgentTask) Stop() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// Kill forcefully terminates the task
func (t *AgentTask) Kill() error {
	if t.cancel != nil {
		t.cancel()
	}
	t.MarkKilled()
	return nil
}

// GetStatus returns the current task status info
func (t *AgentTask) GetStatus() TaskInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TaskInfo{
		ID:          t.ID,
		Type:        TaskTypeAgent,
		Description: t.Description,
		Status:      t.Status,
		StartTime:   t.StartTime,
		EndTime:     t.EndTime,
		Error:       t.Error,
		Output:      t.output.String(),
		AgentName:   t.AgentName,
		TurnCount:   t.TurnCount,
		TokenUsage:  t.TokenUsage,
	}
}

// UpdateProgress updates the turn count and token usage
func (t *AgentTask) UpdateProgress(turnCount, tokenUsage int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TurnCount = turnCount
	t.TokenUsage = tokenUsage
}

// GetContext returns the task's context
func (t *AgentTask) GetContext() context.Context {
	return t.ctx
}

// GetCancel returns the task's cancel function
func (t *AgentTask) GetCancel() context.CancelFunc {
	return t.cancel
}

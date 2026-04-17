package task

import (
	"sync"

	"github.com/yanmxa/gencode/internal/hook"
)

// CompletionObserver receives task lifecycle notifications for app-layer
// concerns (tracker updates, notification queue). Hook firing is handled
// directly by this package via hook.DefaultEngine.
type CompletionObserver interface {
	TaskCreated(info TaskInfo)
	TaskCompleted(info TaskInfo)
}

var taskObserver struct {
	mu       sync.RWMutex
	observer CompletionObserver
}

// SetCompletionObserver installs or clears the task completion observer.
func SetCompletionObserver(observer CompletionObserver) {
	taskObserver.mu.Lock()
	defer taskObserver.mu.Unlock()
	taskObserver.observer = observer
}

func notifyTaskCreated(info TaskInfo) {
	subject := taskSubject(info)
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.TaskCreated, hook.HookInput{
			TaskID:          info.ID,
			TaskSubject:     subject,
			TaskDescription: info.Description,
		})
	}
	taskObserver.mu.RLock()
	obs := taskObserver.observer
	taskObserver.mu.RUnlock()
	if obs != nil {
		obs.TaskCreated(info)
	}
}

func notifyTaskCompleted(info TaskInfo) {
	subject := taskSubject(info)
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.TaskCompleted, hook.HookInput{
			TaskID:          info.ID,
			TaskSubject:     subject,
			TaskDescription: info.Description,
		})
	}
	taskObserver.mu.RLock()
	obs := taskObserver.observer
	taskObserver.mu.RUnlock()
	if obs != nil {
		obs.TaskCompleted(info)
	}
}

func taskSubject(info TaskInfo) string {
	switch info.Type {
	case TaskTypeAgent:
		if info.AgentName != "" && info.Description != "" {
			return info.AgentName + ": " + info.Description
		}
		if info.AgentName != "" {
			return info.AgentName
		}
	case TaskTypeBash:
		if info.Command != "" {
			return info.Command
		}
	}
	return info.Description
}

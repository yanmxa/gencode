package task

import (
	"sync"
)

// CompletionObserver receives task lifecycle notifications. The app layer
// installs an observer that handles tracker updates, notification queue
// pushes, and hook firing.
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
	taskObserver.mu.RLock()
	obs := taskObserver.observer
	taskObserver.mu.RUnlock()
	if obs != nil {
		obs.TaskCreated(info)
	}
}

func notifyTaskCompleted(info TaskInfo) {
	taskObserver.mu.RLock()
	obs := taskObserver.observer
	taskObserver.mu.RUnlock()
	if obs != nil {
		obs.TaskCompleted(info)
	}
}

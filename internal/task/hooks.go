package task

import (
	"sync"
)

// LifecycleHandler receives task lifecycle notifications. The app layer
// installs a handler that fires hooks, updates the tracker, and publishes
// events to the Hub.
type LifecycleHandler interface {
	TaskCreated(info TaskInfo)
	TaskCompleted(info TaskInfo)
}

var taskHandler struct {
	mu      sync.RWMutex
	handler LifecycleHandler
}

// SetLifecycleHandler installs or clears the task lifecycle handler.
func SetLifecycleHandler(h LifecycleHandler) {
	taskHandler.mu.Lock()
	defer taskHandler.mu.Unlock()
	taskHandler.handler = h
}

func notifyTaskCreated(info TaskInfo) {
	taskHandler.mu.RLock()
	h := taskHandler.handler
	taskHandler.mu.RUnlock()
	if h != nil {
		h.TaskCreated(info)
	}
}

func notifyTaskCompleted(info TaskInfo) {
	taskHandler.mu.RLock()
	h := taskHandler.handler
	taskHandler.mu.RUnlock()
	if h != nil {
		h.TaskCompleted(info)
	}
}

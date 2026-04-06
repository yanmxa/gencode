package task

import "sync"

// HookObserver receives task lifecycle notifications without coupling the task
// package to the hooks runtime.
type HookObserver interface {
	TaskCreated(info TaskInfo)
	TaskCompleted(info TaskInfo)
}

var taskHookObserver struct {
	mu       sync.RWMutex
	observer HookObserver
}

// SetHookObserver installs or clears the current task hook observer.
func SetHookObserver(observer HookObserver) {
	taskHookObserver.mu.Lock()
	defer taskHookObserver.mu.Unlock()
	taskHookObserver.observer = observer
}

func notifyTaskCreated(info TaskInfo) {
	taskHookObserver.mu.RLock()
	observer := taskHookObserver.observer
	taskHookObserver.mu.RUnlock()
	if observer != nil {
		observer.TaskCreated(info)
	}
}

func notifyTaskCompleted(info TaskInfo) {
	taskHookObserver.mu.RLock()
	observer := taskHookObserver.observer
	taskHookObserver.mu.RUnlock()
	if observer != nil {
		observer.TaskCompleted(info)
	}
}

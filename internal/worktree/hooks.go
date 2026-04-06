package worktree

import "sync"

// HookObserver receives worktree lifecycle notifications without coupling the
// worktree package to the hooks runtime.
type HookObserver interface {
	WorktreeCreated(name, path string)
	WorktreeRemoved(path string)
}

var worktreeHookObserver struct {
	mu       sync.RWMutex
	observer HookObserver
}

// SetHookObserver installs or clears the current worktree hook observer.
func SetHookObserver(observer HookObserver) {
	worktreeHookObserver.mu.Lock()
	defer worktreeHookObserver.mu.Unlock()
	worktreeHookObserver.observer = observer
}

func notifyWorktreeCreated(name, path string) {
	worktreeHookObserver.mu.RLock()
	observer := worktreeHookObserver.observer
	worktreeHookObserver.mu.RUnlock()
	if observer != nil {
		observer.WorktreeCreated(name, path)
	}
}

func notifyWorktreeRemoved(path string) {
	worktreeHookObserver.mu.RLock()
	observer := worktreeHookObserver.observer
	worktreeHookObserver.mu.RUnlock()
	if observer != nil {
		observer.WorktreeRemoved(path)
	}
}

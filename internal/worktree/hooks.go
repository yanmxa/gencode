package worktree

import "github.com/yanmxa/gencode/internal/hook"

func fireWorktreeCreated(name, path string) {
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.WorktreeCreate, hook.HookInput{
			Name:         name,
			WorktreePath: path,
		})
	}
}

func fireWorktreeRemoved(path string) {
	if hook.DefaultEngine != nil {
		hook.DefaultEngine.ExecuteAsync(hook.WorktreeRemove, hook.HookInput{
			WorktreePath: path,
		})
	}
}

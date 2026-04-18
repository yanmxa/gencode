package worktree

import "github.com/yanmxa/gencode/internal/hook"

func fireWorktreeCreated(name, path string) {
	if h := hook.DefaultIfInit(); h != nil {
		h.ExecuteAsync(hook.WorktreeCreate, hook.HookInput{
			Name:         name,
			WorktreePath: path,
		})
	}
}

func fireWorktreeRemoved(path string) {
	if h := hook.DefaultIfInit(); h != nil {
		h.ExecuteAsync(hook.WorktreeRemove, hook.HookInput{
			WorktreePath: path,
		})
	}
}

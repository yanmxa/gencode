package worktree

import (
	"testing"

	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/setting"
)

func TestWorktreeHooksFire(t *testing.T) {
	hook.SetDefault(hook.NewEngine(&setting.Settings{}, "test", t.TempDir(), ""))
	defer hook.ResetService()

	repo := makeRepo(t)

	result, _, err := Create(repo, "hook-test")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := Remove(repo, result.Path); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

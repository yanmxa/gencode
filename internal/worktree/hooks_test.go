package worktree

import "testing"

type testWorktreeObserver struct {
	createdNames []string
	createdPaths []string
	removedPaths []string
}

func (o *testWorktreeObserver) WorktreeCreated(name, path string) {
	o.createdNames = append(o.createdNames, name)
	o.createdPaths = append(o.createdPaths, path)
}

func (o *testWorktreeObserver) WorktreeRemoved(path string) {
	o.removedPaths = append(o.removedPaths, path)
}

func TestWorktreeHookObserver(t *testing.T) {
	repo := makeRepo(t)
	observer := &testWorktreeObserver{}
	SetHookObserver(observer)
	defer SetHookObserver(nil)

	result, _, err := Create(repo, "hook-observer")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if len(observer.createdNames) != 1 || observer.createdNames[0] != "hook-observer" {
		t.Fatalf("expected created observer for hook-observer, got %v", observer.createdNames)
	}
	if len(observer.createdPaths) != 1 || observer.createdPaths[0] != result.Path {
		t.Fatalf("expected created path %q, got %v", result.Path, observer.createdPaths)
	}

	if err := Remove(repo, result.Path); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if len(observer.removedPaths) != 1 || observer.removedPaths[0] != result.Path {
		t.Fatalf("expected removed path %q, got %v", result.Path, observer.removedPaths)
	}
}

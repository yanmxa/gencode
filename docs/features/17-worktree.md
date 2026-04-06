# Feature 17: Worktree System

## Overview

Git worktrees allow the LLM to work on an isolated branch and file system without affecting the main working directory. Each worktree has its own independent context.

**Tool interface:**

| Tool | Parameters |
|------|-----------|
| `EnterWorktree` | `branch`, `path` |
| `ExitWorktree` | `keep=true\|false` |

- `keep=true` — retain the worktree and branch after exit
- `keep=false` — delete the worktree and its branch after exit

## UI Interactions

- `EnterWorktree` shows a tool call card with the branch name and path.
- While inside a worktree, the status bar may indicate the active branch.
- `ExitWorktree` shows a confirmation card with the keep/delete decision.

## Automated Tests

```bash
go test ./internal/worktree/... -v
```

Cases to add:

```go
func TestWorktree_CreateAndEnter(t *testing.T) {
    // EnterWorktree must create a valid git worktree at the specified path
}

func TestWorktree_Exit_Remove(t *testing.T) {
    // ExitWorktree keep=false must delete the worktree and its branch
}

func TestWorktree_Exit_Keep(t *testing.T) {
    // ExitWorktree keep=true must leave the worktree intact
}

func TestWorktree_RequiresGitRepo(t *testing.T) {
    // EnterWorktree outside a git repo must return a descriptive error
}

func TestWorktree_BranchParam_CreatesBranch(t *testing.T) {
    // EnterWorktree with branch param must create the specified branch
}

func TestWorktree_PathParam_UsesPath(t *testing.T) {
    // EnterWorktree with path param must create worktree at that path
}

func TestWorktree_StatusBar_ShowsBranch(t *testing.T) {
    // Status bar must indicate the active branch while in worktree
}
```

## Interactive Tests (tmux)

```bash
# Ensure we have a git repo to test with
mkdir -p /tmp/wt_test && cd /tmp/wt_test
git init && echo "test" > file.txt && git add . && git commit -m "init"

tmux new-session -d -s t_wt -x 220 -y 60
tmux send-keys -t t_wt 'cd /tmp/wt_test && gen' Enter
sleep 2

# Test 1: Create a worktree
tmux send-keys -t t_wt 'enter a git worktree for branch test-worktree-branch' Enter
sleep 8
tmux capture-pane -t t_wt -p
# Expected: EnterWorktree tool called; worktree created

# Test 2: Verify worktree exists
git -C /tmp/wt_test worktree list
# Expected: new worktree listed with branch test-worktree-branch

# Test 3: Exit and remove the worktree (keep=false)
tmux send-keys -t t_wt 'exit the worktree and remove it' Enter
sleep 5
tmux capture-pane -t t_wt -p
git -C /tmp/wt_test worktree list
# Expected: worktree no longer listed

# Test 4: Create and keep worktree (keep=true)
tmux send-keys -t t_wt 'enter a git worktree for branch keep-branch' Enter
sleep 8
tmux send-keys -t t_wt 'exit the worktree but keep it' Enter
sleep 5
git -C /tmp/wt_test worktree list
# Expected: worktree still listed with branch keep-branch

# Test 5: Confirmation card on exit
tmux send-keys -t t_wt 'enter a git worktree for branch confirm-test' Enter
sleep 8
tmux send-keys -t t_wt 'exit the worktree' Enter
sleep 3
tmux capture-pane -t t_wt -p
# Expected: confirmation card shows keep/delete decision

tmux kill-session -t t_wt
rm -rf /tmp/wt_test
```

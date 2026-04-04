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
```

## Interactive Tests (tmux)

```bash
cd /home/cloud-user/workspace/gencode

tmux new-session -d -s t_wt -x 220 -y 60
tmux send-keys -t t_wt 'cd /home/cloud-user/workspace/gencode && gen' Enter
sleep 2

# Create a worktree
tmux send-keys -t t_wt 'enter a git worktree for branch test-worktree-branch' Enter
sleep 8
tmux capture-pane -t t_wt -p
# Expected: EnterWorktree tool called; worktree created

git worktree list
# Expected: new worktree listed

# Exit and remove the worktree
tmux send-keys -t t_wt 'exit the worktree and remove it' Enter
sleep 5
tmux capture-pane -t t_wt -p
git worktree list
# Expected: worktree no longer listed

tmux kill-session -t t_wt
```

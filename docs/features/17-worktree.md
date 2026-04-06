# Feature 17: Worktree System

## Overview

Git worktrees allow the LLM to work on an isolated branch and file system without affecting the main working directory. Each worktree has its own independent context.

**Tool interface:**

| Tool | Parameters |
|------|-----------|
| `EnterWorktree` | `name` (optional slug) |
| `ExitWorktree` | `action=keep\|remove`, `discard_changes` |

- `action=keep` — retain the worktree for later reuse
- `action=remove` — delete the worktree; if dirty, require `discard_changes=true`

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
    // EnterWorktree must create a valid git worktree using the generated or requested slug
}

func TestWorktree_Exit_Remove(t *testing.T) {
    // ExitWorktree action=remove must delete the worktree and its branch
}

func TestWorktree_Exit_Keep(t *testing.T) {
    // ExitWorktree action=keep must leave the worktree intact
}

func TestWorktree_RequiresGitRepo(t *testing.T) {
    // EnterWorktree outside a git repo must return a descriptive error
}

func TestWorktree_NameParam_UsesSlug(t *testing.T) {
    // EnterWorktree with name param must create a worktree directory using that slug
}

func TestWorktree_DiscardChanges_RequiredForDirtyRemove(t *testing.T) {
    // Removing a dirty worktree must require discard_changes=true
}

func TestWorktree_StatusBar_ShowsBranch(t *testing.T) {
    // Status bar must indicate the active branch while in worktree
}
```

## Interactive Tests (tmux)

```bash
# Ensure we have a git repo to test with
mkdir -p /tmp/wt_test && cd /tmp/wt_test
git init
git config user.name "GenCode Test"
git config user.email "test@example.com"
echo "test" > file.txt && git add . && git commit -m "init"

tmux new-session -d -s t_wt -x 220 -y 60
tmux send-keys -t t_wt 'cd /tmp/wt_test && gen' Enter
sleep 2

# Test 1: Create a worktree
tmux send-keys -t t_wt 'enter a git worktree named test-worktree' Enter
sleep 8
tmux capture-pane -t t_wt -p
# Expected: EnterWorktree flow shown; worktree path appears in the result card

# Test 2: Verify worktree exists
git -C /tmp/wt_test worktree list
# Expected: new worktree listed with slug test-worktree

# Test 3: Exit and remove the worktree (action=remove)
tmux send-keys -t t_wt 'exit the worktree and remove it' Enter
sleep 5
tmux capture-pane -t t_wt -p
git -C /tmp/wt_test worktree list
# Expected: worktree no longer listed

# Test 4: Create and keep worktree (action=keep)
tmux send-keys -t t_wt 'enter a git worktree named keep-branch' Enter
sleep 8
tmux send-keys -t t_wt 'exit the worktree but keep it' Enter
sleep 5
git -C /tmp/wt_test worktree list
# Expected: worktree still listed with slug keep-branch

# Test 5: Confirmation card on exit
tmux send-keys -t t_wt 'enter a git worktree named confirm-test' Enter
sleep 8
tmux send-keys -t t_wt 'exit the worktree' Enter
sleep 3
tmux capture-pane -t t_wt -p
# Expected: ExitWorktree confirmation shows the keep/remove decision

# Test 6: Dirty worktree requires discard confirmation
tmux send-keys -t t_wt 'edit file.txt and leave it uncommitted in the worktree' Enter
sleep 5
tmux send-keys -t t_wt 'exit the worktree and remove it' Enter
sleep 3
tmux capture-pane -t t_wt -p
# Expected: exit flow asks to discard uncommitted changes before removal

tmux kill-session -t t_wt
rm -rf /tmp/wt_test
```

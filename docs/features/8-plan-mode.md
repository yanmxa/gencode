# Feature 8: Plan Mode

## Overview

Plan mode restricts the LLM to read-only exploration. Write operations are blocked until the user explicitly approves exiting the mode.

**Allowed tools in plan mode:** Read, Glob, Grep, WebFetch, WebSearch

**Blocked in plan mode:** Write, Edit, Bash (write-capable), Skills

**Entry:**
- CLI flag: `gen --plan "task description"`
- Slash command: `/plan`

**Exit:** LLM calls `ExitPlanMode` tool → user sees a plan summary and must approve to continue with write access.

## UI Interactions

- Status bar shows `[PLAN MODE]` when active.
- Write tool calls return an inline error message without a permission dialog.
- `ExitPlanMode` approval shows a plan summary; user presses `y` to approve or `n` to stay in plan mode.

## Automated Tests

```bash
go test ./internal/tool/... -v -run TestExitPlanMode
go test ./internal/plan/... -v
```

Covered:

```
TestExitPlanMode_RequiresApproval
TestExitPlanMode_ApprovalGranted
TestExitPlanMode_ApprovalDenied_StaysInPlanMode
```

Cases to add:

```go
func TestPlanMode_BlocksWriteTools(t *testing.T) {
    // Write and Edit must error when plan mode is active
}

func TestPlanMode_AllowsReadTools(t *testing.T) {
    // Read, Glob, Grep must execute normally in plan mode
}

func TestPlanMode_StatusBar_ReflectsMode(t *testing.T) {
    // The UI mode field must equal Plan when active
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_plan -x 220 -y 60

# Start in plan mode via flag
tmux send-keys -t t_plan 'gen --plan "analyze the project layout"' Enter
sleep 2
tmux capture-pane -t t_plan -p
# Expected: [PLAN MODE] in status bar

# Attempt a write — must be rejected
tmux send-keys -t t_plan 'create a file called out.txt' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: error message saying writes are not allowed in plan mode

# Read operation — must succeed
tmux send-keys -t t_plan 'read /etc/hostname' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: hostname content shown

# Enter via /plan command
tmux send-keys -t t_plan 'q' Enter
tmux send-keys -t t_plan 'gen' Enter
sleep 2
tmux send-keys -t t_plan '/plan' Enter
sleep 1
tmux capture-pane -t t_plan -p
# Expected: status bar switches to [PLAN MODE]

# Exit plan mode (requires approval)
tmux send-keys -t t_plan 'done planning, exit plan mode' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: plan summary shown; prompt to approve exit

tmux kill-session -t t_plan
```

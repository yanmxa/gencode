# Feature 8: Plan Mode

## Overview

Plan mode restricts the LLM to read-only exploration. Write operations are blocked until the user explicitly approves exiting the mode.

**Allowed tools in plan mode:** Read, Glob, Grep, WebFetch, WebSearch, AskUserQuestion, plan-mode `Agent`, ExitPlanMode

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
# ExitPlanMode tool
TestExitPlanMode_ModifyKeepsPlanMode        — modify mode keeps plan mode active
TestExitPlanMode_ApprovalModes              — clear-auto, auto, manual modes work
TestExitPlanMode_Rejected                   — rejected plans handled

# Plan mode approval flow (app update)
TestPlanResponse_ModifyStaysInPlanMode      — modify keeps plan mode
TestPlanResponse_ManualExitsPlanMode        — manual approval exits
TestPlanResponse_AutoExitsPlanMode          — auto approval exits
TestPlanResponse_RejectedExitsPlanMode      — rejected plan exits

# Tool filtering in plan mode
TestPlanMode_BlocksWriteTools               — Write, Edit, Bash blocked in plan mode
TestPlanMode_AllowsReadTools                — Read, Glob, Grep, ExitPlanMode available

# Agent plan mode
TestAgent_PlanPermissionMode_BlocksWrites   — agent with plan mode blocks writes
```

Cases to add:

```go
func TestPlanMode_StatusBar_ReflectsMode(t *testing.T) {
    // The UI mode field must equal Plan when plan mode is active
}

func TestPlanMode_WebFetchAllowed(t *testing.T) {
    // WebFetch must be allowed in plan mode
}

func TestPlanMode_WebSearchAllowed(t *testing.T) {
    // WebSearch must be allowed in plan mode
}

func TestPlanMode_SkillsBlocked(t *testing.T) {
    // Skill invocation must be blocked in plan mode
}

func TestPlanMode_EntryViaSlashCommand(t *testing.T) {
    // /plan command must switch to plan mode
}

func TestPlanMode_AskUserQuestionAllowed(t *testing.T) {
    // AskUserQuestion must remain available for requirement clarification in plan mode
}

func TestPlanMode_AgentRestrictedToReadOnlyTypes(t *testing.T) {
    // Agent tool in plan mode must use the plan-mode schema without background execution
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

# AskUserQuestion — must succeed
tmux send-keys -t t_plan 'ask me whether to focus on backend or frontend first' Enter
sleep 4
tmux capture-pane -t t_plan -p
# Expected: a question prompt appears even while plan mode is active

# Enter via /plan command
tmux send-keys -t t_plan C-c
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

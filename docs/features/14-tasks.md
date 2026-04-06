# Feature 14: Task / Background Task System

## Overview

The task system runs shell commands or agents in the background while the user continues interacting with the TUI.

**Task types:**

| Type | Description |
|------|-------------|
| `BashTask` | Background shell command |
| `AgentTask` | Background agent execution |

**Task lifecycle:** Running → Completed / Failed / Killed

**Fields per task:** ID, Type, Description, Status, StartTime, EndTime, Duration, Output, Error, ExitCode (Bash), AgentName, TurnCount, TokenUsage (Agent)

**Keyboard shortcut:** `Ctrl+T` toggles the task panel at the bottom of the TUI.

**Tool interface:**

| Tool | Purpose |
|------|---------|
| `TaskCreate` | Start a background task |
| `TaskGet` | Get details for a task |
| `TaskList` | List all tasks |
| `TaskUpdate` | Update task description |
| `TaskStop` | Kill a running task |
| `TaskOutput` | Stream output from a task |

## UI Interactions

- **Task panel** (`Ctrl+T`): shows all tasks with status badges (Running / Completed / Failed / Killed).
- **Task creation**: LLM calls `TaskCreate`; the task ID is shown in the response.
- **Live output**: `TaskOutput` streams output into the conversation in real time.
- **Stop**: `TaskStop` sends SIGKILL; status updates to Killed.

## Automated Tests

```bash
go test ./internal/task/... -v
go test ./internal/tool/... -v -run TestTaskOutput
```

Covered:

```
# BashTask lifecycle
TestBashTask_Complete                 — task completes successfully
TestBashTask_Failed                   — failed task state captured
TestBashTask_MarkKilled               — killed task state
TestBashTask_AppendAndGetOutput       — output streaming and retrieval
TestBashTask_IsRunning                — running state check
TestBashTask_WaitForCompletion        — wait for task to finish
TestBashTask_WaitForCompletionTimeout — timeout during wait
TestBashTask_GetStatus                — status retrieval
TestBashTask_ConcurrentAccess         — concurrent safety
TestBashTask_StatusRunning            — status while running
TestBashTask_AllStateTransitions      — Running → Completed/Failed/Killed
TestBashTask_ImplementsBackgroundTask — interface compliance

# Task manager
TestManager_CreateAndGet              — create and retrieve tasks
TestManager_GetNotFound               — error on missing task
TestManager_List                      — list all tasks
TestManager_ListRunning               — list running tasks only
TestManager_Remove                    — remove a task
TestManager_Cleanup                   — cleanup old tasks
TestManager_CleanupKeepsRecent        — keeps recent completed tasks
TestManager_CleanupKeepsRunning       — keeps running tasks during cleanup
TestManager_GenerateUniqueIDs         — unique task ID generation
TestManager_RegisterTask              — register external task
TestManager_GetBashTask               — get bash task by ID

# TaskOutput tool
TestTaskOutputTool_StillRunning       — reports running tasks
TestTaskOutputTool_Completed          — reports completed tasks
TestTaskOutputTool_NotFound           — handles missing tasks
TestTaskOutputTool_NonBlocking        — non-blocking mode returns immediately
```

Cases to add:

```go
func TestAgentTask_Execute(t *testing.T) {
    // AgentTask must run an agent in the background and capture output
}

func TestAgentTask_Fields(t *testing.T) {
    // AgentTask must populate AgentName, TurnCount, and TokenUsage fields
}

func TestTaskStop_SendsSIGKILL(t *testing.T) {
    // TaskStop must send SIGKILL and set status to Killed
}

func TestTaskCreate_ReturnsTaskID(t *testing.T) {
    // TaskCreate must return the task ID in the tool result
}

func TestTaskUpdate_ChangesDescription(t *testing.T) {
    // TaskUpdate must update the task description
}

func TestTaskOutput_StreamsWhileRunning(t *testing.T) {
    // TaskOutput must stream output incrementally while task runs
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_task -x 220 -y 60
tmux send-keys -t t_task 'gen' Enter
sleep 2

# Test 1: Create a background task
tmux send-keys -t t_task 'run a background task: sleep 30 and then echo done' Enter
sleep 5
tmux capture-pane -t t_task -p
# Expected: TaskCreate called; task ID shown

# Test 2: List tasks
tmux send-keys -t t_task 'list all tasks' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task shown with "Running" status

# Test 3: Toggle task panel (Ctrl+T)
tmux send-keys -t t_task C-t
sleep 1
tmux capture-pane -t t_task -p
# Expected: task panel appears at the bottom with status badge
tmux send-keys -t t_task C-t
sleep 1
tmux capture-pane -t t_task -p
# Expected: task panel hidden again

# Test 4: Stop the task
tmux send-keys -t t_task 'stop the background task' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task status becomes "Killed"

# Test 5: Get task output after completion
tmux send-keys -t t_task 'run a background task: echo hello-output' Enter
sleep 5
tmux send-keys -t t_task 'get the output of the background task' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: TaskOutput returns "hello-output"

# Test 6: Multiple concurrent tasks
tmux send-keys -t t_task 'run two background tasks: one sleeps 10s, another echoes fast' Enter
sleep 5
tmux send-keys -t t_task 'list all tasks' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: multiple tasks shown with respective statuses

# Test 7: Task panel shows completed status
sleep 12
tmux send-keys -t t_task C-t
sleep 1
tmux capture-pane -t t_task -p
# Expected: completed tasks show "Completed" badge

tmux kill-session -t t_task
```

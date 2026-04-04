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
TestBashTask_Execute
TestBashTask_ExitCode
TestBashTask_Cancel
TestTaskManager_Create
TestTaskManager_List
TestTaskManager_Stop
TestTaskOutput_StreamsOutput
TestTaskOutput_CompletedTask
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_task -x 220 -y 60
tmux send-keys -t t_task 'gen' Enter
sleep 2

# Create a background task
tmux send-keys -t t_task 'run a background task: sleep 30 and then echo done' Enter
sleep 5
tmux capture-pane -t t_task -p
# Expected: TaskCreate called; task ID shown

# List tasks
tmux send-keys -t t_task 'list all tasks' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task shown with "Running" status

# Toggle task panel (Ctrl+T)
tmux send-keys -t t_task C-t
sleep 1
tmux capture-pane -t t_task -p
# Expected: task panel appears at the bottom; Ctrl+T again hides it

# Stop the task
tmux send-keys -t t_task 'stop the background task' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task status becomes "Killed"

tmux kill-session -t t_task
```

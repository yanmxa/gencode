# Feature 13: Cron / Scheduling System

## Overview

The cron system lets the LLM schedule recurring or one-time prompts using standard cron expressions.

**Format:** 5-field cron — `minute hour day-of-month month day-of-week`

**Supported syntax:** `*/5`, `1-10/2`, `1,3,5`, `*`, named values (`jan`, `mon`, …)

**Persistence:** `.gen/scheduled_tasks.json`

**Auto-expiry:** recurring jobs expire after 7 days of inactivity.

**Tool interface:**

| Tool | Parameters |
|------|-----------|
| `CronCreate` | `schedule`, `prompt`, `once` |
| `CronDelete` | `job_id` |
| `CronList` | (none) |

## UI Interactions

- `CronCreate` is called by the LLM when the user asks to schedule something.
- When a job fires, its prompt is injected into the conversation as if the user had typed it.
- Job IDs are shown in the tool result and visible via `CronList`.

## Automated Tests

```bash
go test ./internal/cron/... -v
```

Covered:

```
TestCron_ParseExpression
TestCron_NamedMonths
TestCron_NamedWeekdays
TestCron_StepValues
TestCron_RangeValues
TestCron_NextFireTime
TestCron_Persistence
```

Cases to add:

```go
func TestCron_Expiry_After7Days(t *testing.T) {
    // A recurring job older than 7 days must be automatically removed
}

func TestCron_Once_RemovedAfterFiring(t *testing.T) {
    // A once=true job must be deleted after it fires
}

func TestCron_InvalidExpression_ReturnsError(t *testing.T) {
    // Malformed cron strings must return a descriptive parse error
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cron -x 220 -y 60
tmux send-keys -t t_cron 'gen' Enter
sleep 2

# Create a recurring job
tmux send-keys -t t_cron 'schedule a job every minute that says "tick"' Enter
sleep 5
tmux capture-pane -t t_cron -p
# Expected: CronCreate tool called; job ID returned

# List jobs
tmux send-keys -t t_cron 'list all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: job shown with next-fire time

# Wait for it to fire (~1 minute)
sleep 65
tmux capture-pane -t t_cron -p
# Expected: "tick" prompt was fired by the scheduler

# Delete all jobs
tmux send-keys -t t_cron 'delete all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: CronDelete called; no remaining jobs

tmux kill-session -t t_cron
```

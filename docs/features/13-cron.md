# Feature 13: Cron / Scheduling System

## Overview

The cron system lets the LLM schedule recurring or one-time prompts using standard cron expressions.

**Format:** 5-field cron — `minute hour day-of-month month day-of-week`

**Supported syntax:** `*/5`, `1-10/2`, `1,3,5`, `*`, named values (`jan`, `mon`, …)

**Persistence:** `.gen/scheduled_tasks.json`

**Auto-expiry:** recurring jobs expire after 7 days of inactivity.

**Idle behavior:** scheduled prompts fire only while the REPL is idle.

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
# Parsing
TestParse                               — cron expression parsing (8 cases)
TestNextAfter                           — next fire time calculation
TestDescribe                            — human-readable description

# Store operations
TestStoreCreateAndList                  — create and list jobs
TestStoreDelete                         — delete a job
TestStoreTick                           — tick fires due jobs
TestStoreMaxJobs                        — max job limit enforced
TestStoreDurable                        — persistence to disk

# Edge cases
TestCron_InvalidExpression_ReturnsError — malformed cron returns descriptive error
TestCron_Once_RemovedAfterFiring        — once=true job deleted after firing
```

Cases to add:

```go
func TestCron_Expiry_After7Days(t *testing.T) {
    // A recurring job older than 7 days must be automatically removed
}

func TestCron_RangeValidation(t *testing.T) {
    // Minutes 0-59, hours 0-23, day 1-31, month 1-12, dow 0-6 enforced
}

func TestCron_PromptInjection_Fires(t *testing.T) {
    // When a job fires, its prompt must be injected into the conversation
}

func TestCron_NextFireTime_InListResult(t *testing.T) {
    // CronList must show next-fire time for each job
}

func TestCron_IdleOnly_FiresWhenNoActiveStream(t *testing.T) {
    // Cron jobs must wait until the session is idle before injecting prompts
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cron -x 220 -y 60
tmux send-keys -t t_cron 'gen' Enter
sleep 2

# Test 1: Create a recurring job
tmux send-keys -t t_cron 'schedule a job every minute that says "tick"' Enter
sleep 5
tmux capture-pane -t t_cron -p
# Expected: CronCreate tool called; job ID returned

# Test 2: List jobs
tmux send-keys -t t_cron 'list all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: job shown with next-fire time

# Test 3: Wait for it to fire (~1 minute)
sleep 65
tmux capture-pane -t t_cron -p
# Expected: "tick" prompt was fired by the scheduler

# Test 4: Delete all jobs
tmux send-keys -t t_cron 'delete all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: CronDelete called; no remaining jobs

# Test 5: One-time job (once=true)
tmux send-keys -t t_cron 'schedule a one-time job in 1 minute that says "once-fired"' Enter
sleep 5
tmux capture-pane -t t_cron -p
# Expected: CronCreate called with once=true
tmux send-keys -t t_cron 'list all cron jobs' Enter
sleep 3
# Expected: one-time job listed
sleep 65
tmux send-keys -t t_cron 'list all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: one-time job removed after firing

# Test 6: Invalid expression
tmux send-keys -t t_cron 'schedule a job with cron expression "invalid bad" that says test' Enter
sleep 5
tmux capture-pane -t t_cron -p
# Expected: error message about invalid cron expression

# Test 7: Persistence file written
ls /tmp/.gen/scheduled_tasks.json .gen/scheduled_tasks.json 2>/dev/null
# Expected: scheduled_tasks.json exists in the project state directory

tmux kill-session -t t_cron
```

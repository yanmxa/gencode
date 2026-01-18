# Background Bash Testing Guide

## Test Cases

### Test 1: Simple Background Command
**Command:** Run `sleep 3 && echo "Task completed"` in background
**Expected:**
- Task starts and returns task ID
- Can check status with TaskOutput
- Output file contains "Task completed" after 3 seconds

### Test 2: List Tasks Command
**Command:** `/tasks`
**Expected:**
- Shows all background tasks with table header (STATUS, ID, DESCRIPTION, TIME, TYPE)
- Color-coded status indicators:
  - Running: Blue (●) with "Running" label
  - Pending: Gray (○) with "Pending" label
  - Completed: Green (✔) with "Done" label
  - Failed: Red (✖) with "Failed" label
  - Cancelled: Yellow (⊘) with "Stopped" label
- Task ID shown in primary color (first 8 characters)
- Time format: "Xm Ys" for better readability
- Task type auto-detected (bash, test, build, task)

### Test 3: Failed Command
**Command:** Run `invalid-command-xyz` in background
**Expected:**
- Task starts
- Task completes with error status
- Error message indicates command not found

### Test 4: Long Output Command
**Command:** Run `for i in {1..100}; do echo "Line $i"; done` in background
**Expected:**
- Task completes successfully
- Output file contains all lines
- TaskOutput can retrieve the output

### Test 5: Filter Tasks
**Command:** `/tasks running`
**Expected:**
- Shows only running/pending tasks

**Command:** `/tasks completed`
**Expected:**
- Shows only completed tasks

**Command:** `/tasks error`
**Expected:**
- Shows only failed/cancelled tasks

## Manual Test Steps

1. Start the CLI: `npm start`
2. Test background execution:
   - User message: "Run 'sleep 5 && echo done' in background with description 'Test sleep task'"
   - Verify task ID is returned
3. Check tasks list:
   - Command: `/tasks`
   - Verify task appears with running status
4. Wait for completion and check again:
   - Command: `/tasks`
   - Verify task shows completed status
5. Use TaskOutput to check result:
   - User message: "Check the status and result of the task"
   - Verify output contains "done"

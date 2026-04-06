# Feature 3: Tool System (38 Tools)

## Overview

Built-in tools that the LLM can call during a conversation. Tools are executed after permission checks and the results are fed back to the LLM for the next turn.

| Category | Tools |
|----------|-------|
| File read | Read, Glob, Grep |
| File write | Write, Edit |
| Execution | Bash |
| Network | WebFetch, WebSearch |
| Task management | TaskCreate, TaskGet, TaskList, TaskUpdate, TaskStop, TaskOutput |
| Plan mode | EnterPlanMode, ExitPlanMode |
| Worktree | EnterWorktree, ExitWorktree |
| Agent | Agent, AskUserQuestion, Skill |
| Scheduling | CronCreate, CronDelete, CronList |
| System | Set, ToolSearch, SendMessage |
| MCP | ListMcpResourcesTool, ReadMcpResourceTool |

## How Tool Execution Works

1. LLM returns a `tool_use` block with tool name and input JSON.
2. Permission check runs (`allow` / `deny` / `prompt user`).
3. Tool executes; result is added to the conversation.
4. LLM is called again with the tool result.

Tools run in parallel when the LLM returns multiple calls at once (TUI layer). Within the agent core loop they are sequential.

## UI Interactions

- **Permission dialog**: appears when the permission mode requires user confirmation; press `y` to approve or `n` to deny.
- **`/tools` command**: opens a toggle panel listing all tools with enable/disable controls.
- **Streaming tool input**: tool arguments stream into the UI as they are generated.

## Automated Tests

```bash
go test ./internal/tool/... -v
go test ./internal/app/tool/... -v
go test ./internal/config/... -v -run TestBashAST
go test ./internal/task/... -v
```

Covered:

```
# Tool execution framework
TestExecutePreparedToolUsesExecuteApprovedWhenRequested — ExecuteApproved path works
TestExecutePreparedToolUsesExecuteByDefault             — Execute path works
TestExecutePreparedToolRoutesMCPTools                   — MCP tools routed correctly
TestExecutePreparedToolReturnsUnknownToolError           — unknown tool error handling
TestPrepareToolCallParsesAndResolvesBuiltInTool          — built-in tool resolution
TestPrepareToolCallResolvesMCPTool                       — MCP tool resolution
TestExecuteParallelPropagatesContextCancellation         — parallel tool context cancel

# Individual tool tests
TestRead_LineLimit_LargeFile           — Read respects line limit on large files
TestEdit_Fails_WhenOldStringNotUnique  — Edit errors when old_string matches >1 time
TestGlob_PatternMatching               — ** and ? wildcard behavior verified

# ExitPlanMode
TestExitPlanMode_ModifyKeepsPlanMode   — modify mode keeps plan mode active
TestExitPlanMode_ApprovalModes         — clear-auto, auto, manual modes work
TestExitPlanMode_Rejected              — rejected plans handled

# Plan mode tool filtering
TestPlanMode_BlocksWriteTools          — Write, Edit, Bash blocked in plan mode
TestPlanMode_AllowsReadTools           — Read, Glob, Grep, ExitPlanMode available

# TaskOutput
TestTaskOutputTool_StillRunning        — reports running tasks
TestTaskOutputTool_Completed           — reports completed tasks
TestTaskOutputTool_NotFound            — handles missing tasks
TestTaskOutputTool_NonBlocking         — non-blocking mode returns immediately

# BashTask
TestBashTask_Complete                  — task completes successfully
TestBashTask_Failed                    — failed task state
TestBashTask_MarkKilled                — killed task state
TestBashTask_AppendAndGetOutput        — output streaming
TestBashTask_IsRunning                 — running state check
TestBashTask_WaitForCompletion         — wait for completion
TestBashTask_ConcurrentAccess          — concurrent safety

# Task manager
TestManager_CreateAndGet               — create and retrieve tasks
TestManager_List                       — list all tasks
TestManager_ListRunning                — list running tasks only
TestManager_Remove                     — remove task
TestManager_Cleanup                    — cleanup old tasks

# Bash AST security
TestParseBashAST                       — Bash command parsing
TestCheckASTSecurity                   — dangerous command detection
TestCheckASTSecurity_ExcessiveCommands — excessive command blocking
```

Cases to add:

```go
func TestBash_DeniedByPermission(t *testing.T) {
    // Bash blocked when a deny rule matches the command
}

func TestWebFetch_ReturnsContent(t *testing.T) {
    // WebFetch must return page content for a valid URL
}

func TestWebSearch_ReturnsResults(t *testing.T) {
    // WebSearch must return search results
}

func TestEnterWorktree_CreatesWorktree(t *testing.T) {
    // EnterWorktree must create a valid git worktree
}

func TestExitWorktree_RemovesWorktree(t *testing.T) {
    // ExitWorktree keep=false must delete the worktree
}

func TestCronCreate_SchedulesJob(t *testing.T) {
    // CronCreate must persist job and return job_id
}

func TestAskUserQuestion_ReturnsAnswer(t *testing.T) {
    // AskUserQuestion must inject question and capture response
}

func TestToolSearch_FindsTools(t *testing.T) {
    // ToolSearch must return matching tools for a query
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_tools -x 220 -y 60

# Test 1: Bash tool with permission prompt
tmux send-keys -t t_tools 'gen -p "run: echo hello world"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: permission dialog (or auto-execute); output "hello world"

# Test 2: Read tool
tmux send-keys -t t_tools 'gen -p "read /etc/hostname"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: hostname content shown

# Test 3: Write tool
tmux send-keys -t t_tools 'gen -p "create /tmp/gentest.txt with content hello"' Enter
sleep 8
cat /tmp/gentest.txt
# Expected: file contains "hello"

# Test 4: /tools toggle panel
tmux send-keys -t t_tools 'gen' Enter
sleep 2
tmux send-keys -t t_tools '/tools' Enter
sleep 2
tmux capture-pane -t t_tools -p
# Expected: full tool list with enable/disable toggles

# Test 5: Edit tool — modify existing file
echo "old content" > /tmp/gentest_edit.txt
tmux send-keys -t t_tools 'q' Enter
tmux send-keys -t t_tools 'gen -p "edit /tmp/gentest_edit.txt: replace old with new"' Enter
sleep 8
cat /tmp/gentest_edit.txt
# Expected: file contains "new content"

# Test 6: Glob tool — search files
tmux send-keys -t t_tools 'gen -p "find all .go files in the current directory using glob"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: list of .go files

# Test 7: Grep tool — search file contents
tmux send-keys -t t_tools 'gen -p "search for func main in .go files"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: matching lines with file paths

# Test 8: Permission denied — deny rule blocks Bash
mkdir -p /tmp/tools_deny_test/.gen
cat > /tmp/tools_deny_test/.gen/settings.json << 'EOF'
{"permissions": {"deny": ["Bash(rm*)"]}}
EOF
tmux send-keys -t t_tools 'cd /tmp/tools_deny_test && gen' Enter
sleep 2
tmux send-keys -t t_tools 'run: rm -f /tmp/gentest.txt' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: Bash tool blocked by deny rule

tmux kill-session -t t_tools
rm -f /tmp/gentest.txt /tmp/gentest_edit.txt
rm -rf /tmp/tools_deny_test
```

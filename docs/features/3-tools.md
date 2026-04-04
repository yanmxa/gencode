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
```

Covered:

```
internal/tool/execute_test.go       — tool execution framework
internal/tool/exitplanmode_test.go  — ExitPlanMode approval flow
internal/tool/taskoutput_test.go    — TaskOutput streaming
internal/config/bash_ast_test.go    — dangerous Bash command detection
```

Cases to add:

```go
func TestRead_LineLimit_LargeFile(t *testing.T) {
    // Read must respect the line limit on large files
}

func TestEdit_Fails_WhenOldStringNotUnique(t *testing.T) {
    // Edit must error when old_string matches more than once
}

func TestGlob_PatternMatching(t *testing.T) {
    // Verify ** and ? wildcard behavior
}

func TestBash_DeniedByPermission(t *testing.T) {
    // Bash blocked when a deny rule matches the command
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_tools -x 220 -y 60

# Test: Bash tool with permission prompt
tmux send-keys -t t_tools 'gen -p "run: echo hello world"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: permission dialog (or auto-execute); output "hello world"

# Test: Read tool
tmux send-keys -t t_tools 'gen -p "read /etc/hostname"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: hostname content shown

# Test: Write tool
tmux send-keys -t t_tools 'gen -p "create /tmp/gentest.txt with content hello"' Enter
sleep 8
cat /tmp/gentest.txt
# Expected: file contains "hello"

# Test: /tools toggle panel
tmux send-keys -t t_tools 'gen' Enter
sleep 2
tmux send-keys -t t_tools '/tools' Enter
sleep 2
tmux capture-pane -t t_tools -p
# Expected: full tool list with enable/disable toggles

tmux kill-session -t t_tools
rm -f /tmp/gentest.txt
```

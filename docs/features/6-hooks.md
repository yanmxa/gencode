# Feature 6: Hooks System

## Overview

Hooks execute shell commands in response to events in the agent lifecycle. They can log activity, block tool calls, or modify tool input.

**13 event types:**

| Event | When fired |
|-------|-----------|
| `SessionStart` | Session initializes |
| `UserPromptSubmit` | User submits a message |
| `PreToolUse` | Before a tool runs |
| `PermissionRequest` | During permission check |
| `PostToolUse` | After successful tool execution |
| `PostToolUseFailure` | After tool execution error |
| `Notification` | Notification sent |
| `SubagentStart` | Subagent starts |
| `SubagentStop` | Subagent finishes |
| `Stop` | Session stops |
| `PreCompact` | Before compaction |
| `PostCompact` | After compaction |
| `SessionEnd` | Session ends |

**Hook options:**

| Field | Description |
|-------|-------------|
| `type: "command"` | Shell command to run |
| `async: true` | Non-blocking (fire-and-forget) |
| `timeout` | Max execution time in ms |
| `once: true` | Execute only once per session |
| `matcher` | Tool name or event subtype to filter on |

**I/O protocol:**
- stdin → JSON with `session_id`, `tool_name`, `tool_input`, etc.
- stdout → JSON decision: `continue`, `block`, or `updatedInput`
- `exit 2` → block the tool call

## UI Interactions

- Blocked tool calls show an error message with the hook's stderr output.
- Modified tool input (via `updatedInput`) is applied silently before execution.
- Async hooks do not affect the UI response time.

## Automated Tests

```bash
go test ./tests/integration/hooks/... -v
go test ./internal/hooks/... -v
```

Covered:

```
TestHooks_BlockToolCall         — PreToolUse exit 2 blocks the tool
TestHooks_ModifyToolInput       — PreToolUse returns updatedInput
TestHooks_PostToolUse           — fires after success
TestHooks_AsyncExecution        — does not block the main loop
TestHooks_SessionStart          — fires on startup
TestHooks_UserPromptSubmit      — fires on user message
TestHooks_PermissionRequest     — influences permission decision
```

Cases to add:

```go
func TestHooks_Timeout_TerminatesHook(t *testing.T) {
    // A hook exceeding its timeout must be killed; main loop continues
}

func TestHooks_Once_ExecutesExactlyOnce(t *testing.T) {
    // once:true hook must not fire on subsequent triggers
}

func TestHooks_Matcher_ToolNameWildcard(t *testing.T) {
    // Matcher "Bash" must not match "BashTask"
}

func TestHooks_InputContains_SessionContext(t *testing.T) {
    // Hook stdin must include session_id and cwd
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/hook_test/.gen

# Logging hook config
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "SessionStart": [{
      "hooks": [{"type": "command",
        "command": "echo '[hook] session started' >> /tmp/hook_log.txt"}]
    }],
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '[hook] bash pre-use' >> /tmp/hook_log.txt"}]
    }]
  }
}
EOF

tmux new-session -d -s t_hooks -x 220 -y 60
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 3
cat /tmp/hook_log.txt
# Expected: "[hook] session started"

tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 6
cat /tmp/hook_log.txt
# Expected: "[hook] bash pre-use"

# Blocking hook
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo 'blocked by policy' >&2; exit 2"}]
    }]
  }
}
EOF

tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool blocked; "blocked by policy" shown to user

tmux kill-session -t t_hooks
rm -rf /tmp/hook_test /tmp/hook_log.txt
```

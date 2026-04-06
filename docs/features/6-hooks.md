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
# Integration tests
TestHooks_BlockToolCall         — PreToolUse exit 2 blocks the tool
TestHooks_ModifyToolInput       — PreToolUse returns updatedInput
TestHooks_NoHooks_PassThrough   — no hooks configured passes through
TestHooks_NilSettings           — nil settings handled gracefully
TestHooks_HasHooks              — hook presence detection

# Engine tests
TestEngineNoHooks               — engine with no hooks
TestEngineNilSettings            — engine with nil settings
TestEngineHasHooks               — engine has hooks detection
TestEngineMatcherFiltering       — matcher filters events correctly
TestEngineBlockingHook           — blocking hook via exit code
TestEngineJSONBlockingOutput     — JSON block output parsing
TestEngineUpdatedInput           — input modification via stdout
TestEngineEnvironmentVariables   — env vars passed to hooks
TestEnginePermissionMode         — permission mode in hook context

# Hook options
TestHooks_Timeout_TerminatesHook     — hook exceeding timeout is killed; main loop continues
TestHooks_Once_ExecutesExactlyOnce   — once:true hook fires only once per session
TestHooks_InputContains_SessionContext — hook stdin includes session_id and cwd

# Matcher
TestMatchesEvent                — event type matching
TestGetMatchValue               — match value extraction
TestEventSupportsMatcher        — events that support matchers
```

Cases to add:

```go
func TestHooks_Matcher_ToolNameWildcard(t *testing.T) {
    // Matcher "Bash" must not match "BashTask"
}

func TestHooks_PostToolUseFailure_Fires(t *testing.T) {
    // PostToolUseFailure must fire when tool execution errors
}

func TestHooks_SubagentStart_Fires(t *testing.T) {
    // SubagentStart must fire when a subagent is spawned
}

func TestHooks_SubagentStop_Fires(t *testing.T) {
    // SubagentStop must fire when a subagent completes
}

func TestHooks_PreCompact_Fires(t *testing.T) {
    // PreCompact must fire before compaction runs
}

func TestHooks_PostCompact_Fires(t *testing.T) {
    // PostCompact must fire after compaction completes
}

func TestHooks_SessionEnd_Fires(t *testing.T) {
    // SessionEnd must fire when the session terminates
}

func TestHooks_Stop_Fires(t *testing.T) {
    // Stop event must fire when session stops
}

func TestHooks_AsyncExecution_DoesNotBlock(t *testing.T) {
    // Async hooks must not block the main loop
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/hook_test/.gen

# Test 1: Logging hook — SessionStart and PreToolUse
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
# Expected: "[hook] bash pre-use" appended

# Test 2: Blocking hook — PreToolUse exit 2
tmux send-keys -t t_hooks 'q' Enter
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
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool blocked; "blocked by policy" shown to user

# Test 3: Input modification hook — updatedInput
tmux send-keys -t t_hooks 'q' Enter
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '{\"decision\":\"updatedInput\",\"updatedInput\":{\"command\":\"echo modified\"}}'"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo original using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: "modified" output instead of "original"

# Test 4: PostToolUse hook fires after success
tmux send-keys -t t_hooks 'q' Enter
rm -f /tmp/hook_log.txt
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PostToolUse": [{
      "hooks": [{"type": "command",
        "command": "echo '[hook] post-tool-use' >> /tmp/hook_log.txt"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo hello using bash' Enter
sleep 5
cat /tmp/hook_log.txt
# Expected: "[hook] post-tool-use"

# Test 5: Once hook fires only once
tmux send-keys -t t_hooks 'q' Enter
rm -f /tmp/hook_log.txt
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '[hook] once' >> /tmp/hook_log.txt",
        "once": true}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo first using bash' Enter
sleep 5
tmux send-keys -t t_hooks 'run echo second using bash' Enter
sleep 5
wc -l /tmp/hook_log.txt
# Expected: exactly 1 line (hook fired only once)

tmux kill-session -t t_hooks
rm -rf /tmp/hook_test /tmp/hook_log.txt
```

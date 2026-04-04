# Feature 2: Session & Conversation System

## Overview

Sessions persist conversations to disk as JSONL files. Each session has metadata (title, provider, model, timestamps) and can be resumed, forked, or listed.

| Concept | Detail |
|---------|--------|
| Storage format | JSONL — one JSON object per line |
| Location | `~/.gen/sessions/` or `./.gen/sessions/` |
| Message types | User, Assistant, ToolUse, ToolResult, Notice, Thinking |
| Resume | `-c` (latest), `-r <id>` (specific) |
| Fork | Branch from any session without modifying the original |

## UI Interactions

- **Session picker (`-r`)**: scrollable list ordered by last-update time; select with arrow keys + Enter.
- **Active session**: status bar shows session ID and message count.
- **Fork**: creates a new session that starts with the original history; both sessions are independent afterwards.
- **Streaming**: tokens render in real time as they arrive from the LLM.

## Automated Tests

```bash
go test ./tests/integration/session/... -v
go test ./internal/session/... -v
```

Covered:

```
TestSession_SaveAndLoad
TestSession_MetadataIndex
TestSession_Fork_IsIsolated
TestSession_ListSortedByTime
```

Cases to add:

```go
func TestSession_JSONL_Integrity(t *testing.T) {
    // Every line in the JSONL file must be valid JSON
}

func TestSession_ContinueRestoresMessages(t *testing.T) {
    // -c must replay all messages in correct order
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_sess -x 220 -y 60

# Test 1: Create session and send a message
tmux send-keys -t t_sess 'gen' Enter
sleep 2
tmux send-keys -t t_sess 'hello, remember the number 42' Enter
sleep 8
# Expected: streaming assistant reply visible

# Test 2: Exit and resume
tmux send-keys -t t_sess 'q' Enter
sleep 1
tmux send-keys -t t_sess 'gen -c' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: previous session history visible

# Test 3: Fork session
tmux send-keys -t t_sess 'q' Enter
tmux send-keys -t t_sess 'gen -c --fork' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: new session with original history; original session unchanged

# Test 4: Session list picker
tmux send-keys -t t_sess 'q' Enter
tmux send-keys -t t_sess 'gen -r' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: selectable list ordered by update time

tmux kill-session -t t_sess
```

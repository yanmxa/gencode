# Feature 2: Session & Conversation System

## Overview

Sessions persist conversations to disk as transcript event logs under the project store. Each session keeps message events plus projected state (title, last prompt, provider/model, timestamps, tasks, compaction summary) so it can be resumed, forked, or listed without replaying the TUI runtime manually.

Detailed persistence, recovery, and transcript-specific test coverage now live in `docs/transcriptstore.md`.

| Concept | Detail |
|---------|--------|
| Storage format | Transcript JSONL event log + projected index |
| Location | `~/.gen/projects/<encoded-cwd>/transcripts/`, `transcripts-index.json`, `blobs/` |
| Message types | User, Assistant, ToolUse, ToolResult, Notice, Thinking |
| Resume | `-c` (latest), `-r <id>` (specific) |
| Fork | Branch from any session without modifying the original |
| Session memory | Compaction summary persisted into transcript state and reloaded with the session |

## UI Interactions

- **Session picker (`-r`)**: scrollable list ordered by last-update time; select with arrow keys + Enter.
- **Active session**: status bar shows session ID and message count.
- **Fork**: creates a new session that starts with the original history; both sessions are independent afterwards.
- **Streaming**: tokens render in real time as they arrive from the LLM.

## Automated Tests

```bash
GOCACHE=/tmp/gocache go test ./internal/transcriptstore ./internal/app/session ./tests/integration/session/... ./tests/integration/cli/...
```

Covered:

```
TestSession_SaveAndLoad               — sessions save and load correctly
TestSession_List                      — sessions list sorted by update time, newest first
TestSession_GetLatest                 — GetLatest returns most recent session
TestSession_Delete                    — session deletion works
TestSession_Cleanup                   — old sessions (>30 days) cleaned up
TestSession_AppendBehavior            — multiple saves append entries correctly
TestSession_MetadataUpdatesOnNewMessage — timestamps and message count update
TestSession_JSONL_Integrity           — every line in JSONL is valid JSON
TestSession_ContinueRestoresMessages  — load restores all messages in correct order
TestSession_EntryRoundtrip            — Messages ↔ Entries conversion maintains fidelity
TestSession_MessageTypes_PersistRoundTrip — tool use/result payloads survive save/load
TestSession_PersistToolResult         — large tool results persisted separately
TestSession_SaveAndLoadSessionMemory  — session memory saved/loaded
TestSession_LoadSessionMemory_NotFound — missing memory returns empty
TestSession_SaveSessionMemory_Overwrite — memory overwrites correctly
TestSession_MemoryEndToEnd            — full save → memory save → load → memory load flow
TestSessionFork_IsIndependent         — fork creates independent session with ParentSessionID
```

## Interactive Tests (tmux)

For the full transcript persistence checklist, including `--continue`, `--resume`, `--fork`, compact restore, JSONL validation, and tool-result hydration, use `docs/transcriptstore.md`.

```bash
tmux new-session -d -s t_sess -x 220 -y 60

# Test 1: Create session and send a message with streaming
tmux send-keys -t t_sess 'gen' Enter
sleep 2
tmux send-keys -t t_sess 'hello, remember the number 42' Enter
sleep 8
tmux capture-pane -t t_sess -p
# Expected: streaming assistant reply visible

# Test 2: Exit and resume with -c
tmux send-keys -t t_sess C-c
sleep 1
tmux send-keys -t t_sess 'gen -c' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: previous session history visible; "42" context preserved

# Test 3: Verify resumed context
tmux send-keys -t t_sess 'what number did I ask you to remember?' Enter
sleep 8
tmux capture-pane -t t_sess -p
# Expected: assistant mentions 42

# Test 4: Fork session
tmux send-keys -t t_sess C-c
tmux send-keys -t t_sess 'gen -c --fork' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: new session with original history; original session unchanged

# Test 5: Session list picker
tmux send-keys -t t_sess C-c
tmux send-keys -t t_sess 'gen -r' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: selectable list ordered by update time; navigate with arrows

# Test 6: Select specific session from picker
tmux send-keys -t t_sess Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: selected session loaded with its history

# Test 7: Status bar shows session info
tmux capture-pane -t t_sess -p | tail -3
# Expected: session ID and message count in status bar

# Test 8: Raw transcript JSONL remains valid after interactive usage
PROJECT_DIR=~/.gen/projects/$(pwd | sed 's#/#-#g')
SESSION_FILE=$(find "${PROJECT_DIR}/transcripts" -name '*.jsonl' | head -1)
export SESSION_FILE
python - <<'PY'
import json, pathlib, os
path = pathlib.Path(os.environ["SESSION_FILE"])
for line in path.read_text().splitlines():
    if line.strip():
        json.loads(line)
print("jsonl ok")
PY
# Expected: script prints "jsonl ok"

# Test 9: Session memory is restored on resume after compact
tmux send-keys -t t_sess '/compact remember the 42 example' Enter
sleep 6
tmux send-keys -t t_sess C-c
tmux send-keys -t t_sess 'gen -c' Enter
sleep 2
tmux send-keys -t t_sess 'what did we compact?' Enter
sleep 6
tmux capture-pane -t t_sess -p
# Expected: compacted session summary is still available after resume

tmux kill-session -t t_sess
```

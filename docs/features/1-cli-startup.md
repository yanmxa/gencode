# Feature 1: CLI Entry & Startup Modes

## Overview

`gen` supports several startup modes controlled by flags. The TUI is only launched in interactive mode; other modes produce plain stdout output.

| Flag | Behavior |
|------|----------|
| `gen` | Launch interactive TUI |
| `gen -p "prompt"` | Non-interactive: print response to stdout, no TUI |
| `gen --plan "task"` | Start in plan mode (read-only) |
| `gen -c` | Resume the most recent session |
| `gen -r` | Pick a session from a list |
| `gen -c --fork` | Fork the most recent session |
| `gen -r <id> --fork` | Fork a specific session |
| `gen --plugin-dir PATH` | Load plugins from a directory |
| `gen version` | Print version string |
| `gen help` | Print help |

## UI Interactions

- **Interactive mode**: full TUI with input box, streaming output, and status bar.
- **Print mode (`-p`)**: no TUI; response is written to stdout line by line.
- **Plan mode**: status bar shows `[PLAN MODE]`; write tools are blocked.
- **Session resume (`-r`)**: a scrollable session picker is shown before the TUI starts.

## Automated Tests

```bash
go test ./tests/integration/session/... -v

# Smoke test: non-interactive mode
echo "say hello" | gen -p 2>&1 | grep -i hello

# No provider needed
gen version
gen help
```

Test cases to add:

```go
func TestNonInteractivePrintMode(t *testing.T) {
    // -p must write response to stdout without launching a TUI
}

func TestPlanModeFlag_ToolsAreRestricted(t *testing.T) {
    // --plan flag: write tools must be blocked
}

func TestSessionContinue_LoadsHistory(t *testing.T) {
    // -c must restore previous session messages
}

func TestSessionFork_IsIndependent(t *testing.T) {
    // --fork must create a new session that does not affect the original
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cli -x 200 -y 50

# Test 1: Basic TUI startup
tmux send-keys -t t_cli 'gen' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: TUI appears with input box and status bar

# Test 2: Non-interactive print mode
tmux send-keys -t t_cli 'q' Enter
tmux send-keys -t t_cli 'gen -p "what is 1+1"' Enter
sleep 5
tmux capture-pane -t t_cli -p
# Expected: "2" on stdout; no TUI launched

# Test 3: Plan mode startup
tmux send-keys -t t_cli 'gen --plan "analyze this project"' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: [PLAN MODE] visible in status bar
tmux send-keys -t t_cli 'q' Enter

# Test 4: Session resume picker
tmux send-keys -t t_cli 'gen -r' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: session list sorted by recency; navigate with arrow keys

tmux kill-session -t t_cli
```

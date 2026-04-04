# Feature 4: Slash Commands (18 Commands)

## Overview

Slash commands are typed directly in the TUI input box. They trigger local UI actions or inject structured prompts without sending a regular chat message.

| Command | Function |
|---------|----------|
| `/provider` | Switch LLM provider |
| `/model` | List and select a model |
| `/clear` | Clear chat history |
| `/fork` | Fork the current session |
| `/help` | Show available commands |
| `/glob` | Search files by glob pattern |
| `/tools` | Enable / disable tools |
| `/plan` | Enter plan mode |
| `/skills` | Manage skill states |
| `/agents` | Manage agents |
| `/tokenlimit` | View / set token budget |
| `/compact` | Compress conversation history |
| `/init` | Create GEN.md and config files |
| `/memory` | View / edit memory files |
| `/mcp` | Manage MCP servers |
| `/plugin` | Manage plugins |
| `/think` | Cycle thinking level (off / normal / high / ultra) |

## UI Interactions

- Commands are matched against the registry as the user types; a suggestion dropdown appears.
- Selector commands (`/provider`, `/model`, `/skills`, etc.) open a scrollable picker overlay.
- `/clear` immediately resets the visible conversation.
- `/think` cycles through levels and updates the status bar indicator.

## Automated Tests

```bash
go test ./internal/app/command/... -v
go test ./internal/app/memory/... -v
```

Cases to add:

```go
func TestCommandRegistry_AllCommandsPresent(t *testing.T) {
    // All 18 commands must be registered at startup
}

func TestSlashClear_ResetsConversation(t *testing.T) {
    // /clear must empty the message history
}

func TestSlashInit_CreatesGENmd(t *testing.T) {
    // /init in an empty directory must create .gen/GEN.md
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cmds -x 220 -y 60
tmux send-keys -t t_cmds 'gen' Enter
sleep 2

# /help
tmux send-keys -t t_cmds '/help' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: all 18 commands listed

# /clear
tmux send-keys -t t_cmds 'hello' Enter
sleep 4
tmux send-keys -t t_cmds '/clear' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: blank conversation view

# /think — cycle through levels
tmux send-keys -t t_cmds '/think' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: thinking level options (off / normal / high / ultra)

# /provider
tmux send-keys -t t_cmds '/provider' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: provider selection UI

# /model
tmux send-keys -t t_cmds '/model' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: model list for current provider

# /tokenlimit
tmux send-keys -t t_cmds '/tokenlimit' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: current token usage and limit

# /glob
tmux send-keys -t t_cmds '/glob *.go' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: .go files in cwd listed

# /init — test in a fresh directory
tmux send-keys -t t_cmds 'q' Enter
tmux send-keys -t t_cmds 'mkdir -p /tmp/init_test && cd /tmp/init_test && gen' Enter
sleep 2
tmux send-keys -t t_cmds '/init' Enter
sleep 3
ls /tmp/init_test/.gen/
# Expected: GEN.md created under .gen/

tmux kill-session -t t_cmds
rm -rf /tmp/init_test
```

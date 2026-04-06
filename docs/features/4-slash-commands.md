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

Covered:

```
TestHandlerRegistryMatchesBuiltinCommands — all 18 commands registered
TestExecuteCommandExit                    — /exit returns quit command
TestExecuteCommandUnknown                 — unknown commands show error message
TestHandleInitCommand                     — /init creates .gen/GEN.md file
TestHandleInitCommand (local)             — /init local creates .gen/GEN.local.md
TestHandleInitCommand (rules)             — /init rules creates .gen/rules directory
TestHandleMemoryList                      — /memory list formats output with sections
```

Cases to add:

```go
func TestSlashClear_ResetsConversation(t *testing.T) {
    // /clear must empty the message history
}

func TestSlashFork_CreatesNewSession(t *testing.T) {
    // /fork must create a new independent session with original history
}

func TestSlashCompact_TriggersCompaction(t *testing.T) {
    // /compact must trigger compaction and return summary
}

func TestSlashThink_CyclesLevels(t *testing.T) {
    // /think must cycle off → normal → high → ultra → off
}

func TestSlashProvider_SwitchesPersists(t *testing.T) {
    // /provider selection must switch provider and persist across turns
}

func TestSlashModel_SwitchesModel(t *testing.T) {
    // /model selection must change active model immediately
}

func TestSlashTokenlimit_ShowsUsage(t *testing.T) {
    // /tokenlimit must show current usage and context limit
}

func TestSlashSkills_TogglesState(t *testing.T) {
    // /skills must toggle skill state between disable/enable/active
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cmds -x 220 -y 60
tmux send-keys -t t_cmds 'gen' Enter
sleep 2

# Test 1: /help
tmux send-keys -t t_cmds '/help' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: all 18 commands listed

# Test 2: /clear
tmux send-keys -t t_cmds 'hello' Enter
sleep 4
tmux send-keys -t t_cmds '/clear' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: blank conversation view

# Test 3: /think — cycle through levels
tmux send-keys -t t_cmds '/think' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: thinking level options (off / normal / high / ultra)

# Test 4: /provider
tmux send-keys -t t_cmds '/provider' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: provider selection UI

# Test 5: /model
tmux send-keys -t t_cmds '/model' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: model list for current provider

# Test 6: /tokenlimit
tmux send-keys -t t_cmds '/tokenlimit' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: current token usage and limit

# Test 7: /glob
tmux send-keys -t t_cmds '/glob *.go' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: .go files in cwd listed

# Test 8: /init — test in a fresh directory
tmux send-keys -t t_cmds 'q' Enter
tmux send-keys -t t_cmds 'mkdir -p /tmp/init_test && cd /tmp/init_test && gen' Enter
sleep 2
tmux send-keys -t t_cmds '/init' Enter
sleep 3
ls /tmp/init_test/.gen/
# Expected: GEN.md created under .gen/

# Test 9: Command suggestion dropdown
tmux send-keys -t t_cmds 'q' Enter
tmux send-keys -t t_cmds 'gen' Enter
sleep 2
tmux send-keys -t t_cmds '/pro'
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: suggestion dropdown shows /provider as match

# Test 10: /fork
tmux send-keys -t t_cmds C-c
sleep 0.3
tmux send-keys -t t_cmds 'hello' Enter
sleep 4
tmux send-keys -t t_cmds '/fork' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: new session created with original history

# Test 11: /compact
tmux send-keys -t t_cmds '/compact' Enter
sleep 5
tmux capture-pane -t t_cmds -p
# Expected: compaction triggered; summary shown

# Test 12: /skills
tmux send-keys -t t_cmds '/skills' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: skill list with state toggles

# Test 13: /agents
tmux send-keys -t t_cmds '/agents' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: agent list with enable/disable toggles

# Test 14: /mcp
tmux send-keys -t t_cmds '/mcp' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: MCP management panel

# Test 15: /plugin
tmux send-keys -t t_cmds '/plugin' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: plugin management panel

# Test 16: /memory
tmux send-keys -t t_cmds '/memory' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: loaded memory files displayed

tmux kill-session -t t_cmds
rm -rf /tmp/init_test
```

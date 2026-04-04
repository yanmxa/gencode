# Feature 7: Permission System

## Overview

Controls whether tool calls are allowed, denied, or prompted to the user.

**Permission modes:**

| Mode | Behavior |
|------|----------|
| `Normal` | Prompt user before every tool execution |
| `AutoAccept` | Auto-approve reads and edits |
| `Plan` | Only read-only tools allowed |
| `BypassPermissions` | Auto-approve all (bypass-immune paths still enforced) |
| `DontAsk` | Convert all prompts into automatic denials |

**Rule syntax:** `Tool(pattern)` — e.g. `Bash(npm:*)`, `Read(**/.env)`

- `allow` list — auto-approve matching calls
- `deny` list — block matching calls
- `ask` list — always prompt for matching calls

Working directory enforcement prevents edits outside the project root.

## UI Interactions

- **Confirmation dialog**: shows tool name and input; press `y` to approve, `n` to deny, `a` to allow always.
- **Denied tool**: shows an inline error in the conversation.
- **Allow-list match**: tool runs silently without any dialog.

## Automated Tests

```bash
go test ./internal/config/... -v -run TestPermission
go test ./internal/permission/... -v
go test ./tests/integration/permission/... -v
```

Covered:

```
TestPermission_AllowRule_AutoApproves
TestPermission_DenyRule_Blocks
TestPermission_BashAST_DangerousCommand
TestPermission_WorkDir_OutsideCwd_Blocked
```

Cases to add:

```go
func TestPermission_DontAskMode_DeniesAllPrompts(t *testing.T) {
    // DontAsk must convert every prompt into a denial
}

func TestPermission_BypassPermissions_BypassImmune_Enforced(t *testing.T) {
    // bypass-immune paths must still be blocked in BypassPermissions mode
}

func TestPermission_GlobPattern_MatchesCorrectly(t *testing.T) {
    // ** must match nested paths; *.env must not match subdirectories
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/perm_test

# Test 1: Normal mode — confirmation dialog
tmux new-session -d -s t_perm -x 220 -y 60
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'create file hello.txt with content world' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: permission dialog — press y to approve
tmux send-keys -t t_perm 'y' Enter
sleep 3
cat /tmp/perm_test/hello.txt
# Expected: "world"

# Test 2: Allow list — no prompt
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"allow": ["Write(/tmp/perm_test/*)"]}}
EOF
tmux send-keys -t t_perm 'create file auto.txt with content ok' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: file created without any dialog

# Test 3: Deny list — blocked
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"deny": ["Bash(rm*)"]}}
EOF
tmux send-keys -t t_perm 'run: rm -f /tmp/perm_test/hello.txt' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: Bash blocked by deny rule

tmux kill-session -t t_perm
rm -rf /tmp/perm_test
```

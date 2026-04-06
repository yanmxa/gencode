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
Sensitive paths and destructive commands remain bypass-immune even in `BypassPermissions` mode.

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
# Integration tests
TestPermission_PermitAll_AllowsWrite        — PermitAll mode allows write operations
TestPermission_ReadOnly_BlocksWrite         — ReadOnly mode blocks write operations
TestPermission_ReadOnly_AllowsRead          — ReadOnly mode allows read operations
TestPermission_DenyAll_BlocksEverything     — DenyAll mode blocks all operations
TestPermission_ExecTool_Directly            — direct tool execution path

# Config permission tests
TestMatchRule                               — rule pattern matching (11 sub-tests)
TestBuildRule                               — rule building (5 sub-tests)
TestCheckPermission                         — permission checks (13+ sub-tests)
TestCheckPermissionWithReason               — permission with reason tracking
TestDenialTracking                          — denial fallback mechanism
TestIsDestructiveCommand                    — dangerous command detection (13 sub-tests)
TestIsSensitivePath                         — sensitive path detection (13 sub-tests)
TestSensitivePathsBypassImmune              — bypass-immune bypass rules
TestCheckBashSecurity                       — bash security checks (13 sub-tests)
TestBashSecurityBypassImmune                — bash security bypass-immune

# Permission modes
TestBypassPermissionsMode                   — bypass permissions mode
TestDontAskMode                             — DontAsk converts all prompts to denials
TestDenyRuleBlocksBypass                     — deny rules block bypass mode
TestDenyRulesPriorityOverSession            — deny rules override session allow
TestDestructiveCommandsRequireConfirmation   — destructive commands need confirm
TestWorkingDirectoryConstraint              — edits outside project root blocked
TestSafeToolAllowlist                       — safe tool whitelist
TestPassthroughBehavior                     — passthrough permission behavior
TestResolveHookAllow                        — hook allow resolution
TestOperationModeNext                       — operation mode cycling

# Glob pattern matching
TestPermission_GlobPattern_MatchesCorrectly — ** nested paths, *.env matching
TestIsInWorkingDirectory                    — working directory check
TestNormalizeMacOSPath                      — macOS path normalization
TestIsSubpath                               — subpath detection

# Read-only tool detection
TestIsReadOnlyToolMatchesConfig             — read-only tool identification
TestIsSafeToolMatchesConfig                 — safe tool identification
```

Cases to add:

```go
func TestPermission_AskList_AlwaysPrompts(t *testing.T) {
    // Tools matching ask list must always prompt even in AutoAccept mode
}

func TestPermission_AllowDenyConflict_DenyWins(t *testing.T) {
    // When a tool matches both allow and deny, deny must take precedence
}

func TestPermission_BypassPermissions_BypassImmune_Enforced(t *testing.T) {
    // bypass-immune paths must still be blocked in BypassPermissions mode
}

```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/perm_test/.gen

# Test 1: Normal mode — confirmation dialog (y/n)
tmux new-session -d -s t_perm -x 220 -y 60
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'create file hello.txt with content world' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: permission dialog shows tool name and input; press y to approve
tmux send-keys -t t_perm 'y' Enter
sleep 3
cat /tmp/perm_test/hello.txt
# Expected: "world"

# Test 2: Allow list — auto-approve (no prompt)
tmux send-keys -t t_perm C-c
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"allow": ["Write(/tmp/perm_test/*)"]}}
EOF
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'create file auto.txt with content ok' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: file created without any dialog

# Test 3: Deny list — blocked
tmux send-keys -t t_perm C-c
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"deny": ["Bash(rm*)"]}}
EOF
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'run: rm -f /tmp/perm_test/hello.txt' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: Bash blocked by deny rule; inline error shown

# Test 4: Normal mode — deny with n
tmux send-keys -t t_perm C-c
rm -f /tmp/perm_test/.gen/settings.json
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'run echo secret using bash' Enter
sleep 3
tmux send-keys -t t_perm 'n'
sleep 2
tmux capture-pane -t t_perm -p
# Expected: tool denied; inline error in conversation

# Test 5: Allow always with a
tmux send-keys -t t_perm 'run echo test1 using bash' Enter
sleep 3
tmux send-keys -t t_perm 'a'
sleep 3
tmux send-keys -t t_perm 'run echo test2 using bash' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: second Bash runs without dialog (allow-always applied)

# Test 6: Working directory enforcement
tmux send-keys -t t_perm 'create file /etc/forbidden.txt with content bad' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: blocked — edits outside project root denied

tmux kill-session -t t_perm
rm -rf /tmp/perm_test
```

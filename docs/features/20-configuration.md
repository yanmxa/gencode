# Feature 20: Configuration System

## Overview

Configuration is loaded from multiple files at different scopes. Higher-priority files override lower-priority ones.

**Load priority** (lowest → highest):

| Priority | File |
|----------|------|
| 1 | `~/.claude/settings.json` (Claude user compat) |
| 2 | `~/.gen/settings.json` (GenCode user) |
| 3 | `./.claude/settings.json` (Claude project compat) |
| 4 | `./.gen/settings.json` (GenCode project) |
| 5 | `./.claude/settings.local.json` |
| 6 | `./.gen/settings.local.json` |
| 7 | CLI arguments / environment variables |
| 8 | `managed-settings.json` (read-only system policy) |

**settings.json schema:**

```json
{
  "permissions": {
    "allow": ["Read(**)", "Glob(**)"],
    "deny":  ["Bash(rm -rf*)"],
    "ask":   ["Write(**)"]
  },
  "model": "claude-sonnet-4-6",
  "hooks": { "PreToolUse": [...] },
  "env": { "MY_VAR": "value" },
  "enabledPlugins": { "my-plugin": true },
  "disabledTools": { "WebSearch": true },
  "theme": "dark"
}
```

## UI Interactions

- **`/tools`**: shows which tools are disabled via `disabledTools`.
- **Env vars**: injected into the Bash tool's environment automatically.
- **Theme**: applied at startup; no restart needed when changed via `/provider` or similar commands.

## Automated Tests

```bash
go test ./internal/config/... -v
```

Covered:

```
TestConfig_PermissionPriority
TestConfig_MergeMultipleScopes
TestConfig_WorkDirRestriction
TestConfig_Suggestion
```

Cases to add:

```go
func TestConfig_LocalOverridesProject(t *testing.T) {
    // settings.local.json must override settings.json at the same scope
}

func TestConfig_Env_InjectedIntoBashEnvironment(t *testing.T) {
    // Variables in "env" must be available when Bash executes commands
}

func TestConfig_DisabledTools_HiddenFromModel(t *testing.T) {
    // Tools in disabledTools must not appear in the LLM's tool list
}

func TestConfig_ManagedSettings_ReadOnly(t *testing.T) {
    // managed-settings.json values must not be overridden by user settings
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/cfg_test/.gen

# User-level env var
cat > ~/.gen/settings.json << 'EOF'
{"env": {"SCOPE": "user"}}
EOF

# Project-level overrides it + disables a tool
cat > /tmp/cfg_test/.gen/settings.json << 'EOF'
{"env": {"SCOPE": "project"}, "disabledTools": {"WebSearch": true}}
EOF

tmux new-session -d -s t_cfg -x 220 -y 60
tmux send-keys -t t_cfg 'cd /tmp/cfg_test && gen' Enter
sleep 2

# Verify env override
tmux send-keys -t t_cfg 'run: echo $SCOPE' Enter
sleep 5
tmux capture-pane -t t_cfg -p
# Expected: output is "project" (project config wins)

# Verify disabled tool
tmux send-keys -t t_cfg '/tools' Enter
sleep 2
tmux capture-pane -t t_cfg -p
# Expected: WebSearch shown as disabled

tmux send-keys -t t_cfg 'q' Enter
tmux kill-session -t t_cfg
rm -rf /tmp/cfg_test
```

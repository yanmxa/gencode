# Feature 12: Plugin System

## Overview

Plugins bundle skills, agents, hooks, MCP servers, and LSP servers into a single distributable unit.

**Plugin directory structure:**

```
my-plugin/
├── plugin.json          # Manifest
├── skills/              # Skill directories
├── agents/              # Agent definitions
├── hooks.json           # Hook configurations
├── mcp.json             # MCP servers
└── lsp.json             # LSP servers
```

**plugin.json:**

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Short description",
  "author": { "name": "Author", "email": "", "url": "" }
}
```

**Load scopes:**
- `~/.gen/plugins/` — user-level
- `./.gen/plugins/` — project-level
- `./.gen/plugins-local/` — local (git-ignored)

**CLI commands:**

```bash
gen plugin list
gen plugin validate [path]
gen plugin install <plugin>@<marketplace>
gen plugin uninstall <plugin>
gen plugin enable <plugin>
gen plugin disable <plugin>
gen plugin info <plugin>
```

## UI Interactions

- **`/plugin`**: opens the plugin management panel with installed plugins and their status.
- **`--plugin-dir PATH`**: loads a plugin from a local directory at startup.
- **Plugin skills**: appear in `/skills` and can be invoked as slash commands.

## Automated Tests

```bash
go test ./internal/plugin/... -v
go test ./tests/integration/plugin/... -v
```

Covered:

```
TestPlugin_ManifestParsing
TestPlugin_Validation
TestPlugin_ScopeLoading
TestPlugin_Integration_Enable
TestPlugin_Integration_Disable
TestPlugin_Integration_SkillFromPlugin
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/my-plugin/skills/hello

cat > /tmp/my-plugin/plugin.json << 'EOF'
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Test plugin"
}
EOF

cat > /tmp/my-plugin/skills/hello/skill.md << 'EOF'
---
name: hello
description: Greet from plugin
allowed-tools: []
---

Say "Hello from my-plugin!" and nothing else.
EOF

# Validate
gen plugin validate /tmp/my-plugin
# Expected: validation passes with no errors

# Load via --plugin-dir
tmux new-session -d -s t_plugin -x 220 -y 60
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux send-keys -t t_plugin '/hello' Enter
sleep 5
tmux capture-pane -t t_plugin -p
# Expected: "Hello from my-plugin!"

# List plugins
tmux send-keys -t t_plugin 'q' Enter
tmux send-keys -t t_plugin 'gen plugin list' Enter
sleep 2
tmux capture-pane -t t_plugin -p

# /plugin command in TUI
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux send-keys -t t_plugin '/plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: plugin management UI

tmux kill-session -t t_plugin
rm -rf /tmp/my-plugin
```

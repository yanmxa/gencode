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
# Internal plugin tests
TestExpandPluginRoot                — plugin root path expansion
TestParsePluginRef                  — plugin reference parsing
TestScope                           — plugin scope detection
TestLoadPlugin                      — load plugin from directory
TestRegistry                        — plugin registry operations
TestValidatePlugin                  — plugin validation logic
TestPlugin_Validate_InvalidManifest — invalid manifest detection
TestLoadFromPath                    — load from specific path
TestHooksConfigParsing              — hooks.json parsing
TestMCPConfigParsing                — mcp.json parsing

# Integration tests
TestPluginLoading                   — end-to-end plugin loading
TestPluginComponents                — skill/agent/hooks/mcp components
TestPluginValidation                — validation integration
TestRegistryWithPlugin              — registry with loaded plugin
TestClaudeCodeCompatibility         — Claude Code plugin compat
TestInstalledPluginsV2Format        — v2 format support
TestMarketplaceManager              — marketplace operations
TestPluginAgentPaths                — agent path resolution
```

Cases to add:

```go
func TestPlugin_Install_FromMarketplace(t *testing.T) {
    // gen plugin install must download and install from marketplace
}

func TestPlugin_Uninstall_RemovesPlugin(t *testing.T) {
    // gen plugin uninstall must remove plugin directory
}

func TestPlugin_Enable_ActivatesComponents(t *testing.T) {
    // gen plugin enable must activate skills, hooks, and MCP servers
}

func TestPlugin_Disable_DeactivatesComponents(t *testing.T) {
    // gen plugin disable must deactivate all plugin components
}

func TestPlugin_Info_ShowsDetails(t *testing.T) {
    // gen plugin info must show manifest, components, and status
}

func TestPlugin_ScopeMerge_UserProjectLocal(t *testing.T) {
    // Plugins from user, project, and local scopes must merge correctly
}

func TestPlugin_ConflictResolution_SameSkillName(t *testing.T) {
    // Same skill name in multiple plugins must follow scope priority
}

func TestPlugin_LSPLoading(t *testing.T) {
    // lsp.json in plugin must configure LSP servers
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/my-plugin/.gen-plugin
mkdir -p /tmp/my-plugin/skills/hello

cat > /tmp/my-plugin/.gen-plugin/plugin.json << 'EOF'
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Test plugin"
}
EOF

cat > /tmp/my-plugin/skills/hello/SKILL.md << 'EOF'
---
name: hello
description: Greet from plugin
allowed-tools: []
---

Say "Hello from my-plugin!" and nothing else.
EOF

# Test 1: Validate
tmux new-session -d -s t_plugin -x 220 -y 60
tmux send-keys -t t_plugin 'gen plugin validate /tmp/my-plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: "Plugin validation passed!"

# Test 2: Load via --plugin-dir
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: Gen TUI header appears; plugin is loaded into the session

# Optional if a provider is configured: invoke the plugin skill
# Optional if a provider is configured:
tmux send-keys -t t_plugin '/hello' Enter
sleep 5
tmux capture-pane -t t_plugin -p
# Expected: "Hello from my-plugin!"

# Test 3: List plugins
tmux send-keys -t t_plugin C-c
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin plugin list' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: "my-plugin" listed with description "Test plugin"

# Test 4: /plugin command in TUI
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux send-keys -t t_plugin '/plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: plugin management UI with "Plugin Manager" and "my-plugin" listed

# Test 5: Plugin skill appears in /skills
tmux send-keys -t t_plugin Escape
tmux send-keys -t t_plugin '/skills' Enter
sleep 1
tmux capture-pane -t t_plugin -p
# Expected: "hello" skill listed (from plugin)

# Test 6: Plugin info
tmux send-keys -t t_plugin C-c
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin plugin info my-plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: manifest details, components, and "Skills: 1" shown

tmux kill-session -t t_plugin
rm -rf /tmp/my-plugin
```

package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPluginRoot(t *testing.T) {
	tests := []struct {
		input      string
		pluginPath string
		expected   string
	}{
		{
			input:      "${GEN_PLUGIN_ROOT}/scripts/test.sh",
			pluginPath: "/home/user/plugins/myplugin",
			expected:   "/home/user/plugins/myplugin/scripts/test.sh",
		},
		{
			input:      "${CLAUDE_PLUGIN_ROOT}/config.json",
			pluginPath: "/tmp/plugin",
			expected:   "/tmp/plugin/config.json",
		},
		{
			input:      "no-variables.txt",
			pluginPath: "/tmp/plugin",
			expected:   "no-variables.txt",
		},
	}

	for _, tt := range tests {
		result := ExpandPluginRoot(tt.input, tt.pluginPath)
		if result != tt.expected {
			t.Errorf("ExpandPluginRoot(%q, %q) = %q, want %q",
				tt.input, tt.pluginPath, result, tt.expected)
		}
	}
}

func TestParsePluginRef(t *testing.T) {
	tests := []struct {
		ref         string
		wantName    string
		wantMarket  string
	}{
		{"git@my-plugins", "git", "my-plugins"},
		{"git", "git", ""},
		{"deployment-tools@enterprise", "deployment-tools", "enterprise"},
	}

	for _, tt := range tests {
		name, market := ParsePluginRef(tt.ref)
		if name != tt.wantName || market != tt.wantMarket {
			t.Errorf("ParsePluginRef(%q) = (%q, %q), want (%q, %q)",
				tt.ref, name, market, tt.wantName, tt.wantMarket)
		}
	}
}

func TestScope(t *testing.T) {
	tests := []struct {
		scope    Scope
		str      string
		icon     string
	}{
		{ScopeUser, "user", "👤"},
		{ScopeProject, "project", "📁"},
		{ScopeLocal, "local", "💻"},
		{ScopeManaged, "managed", "🔒"},
	}

	for _, tt := range tests {
		if tt.scope.String() != tt.str {
			t.Errorf("Scope(%q).String() = %q, want %q", tt.scope, tt.scope.String(), tt.str)
		}
		if tt.scope.Icon() != tt.icon {
			t.Errorf("Scope(%q).Icon() = %q, want %q", tt.scope, tt.scope.Icon(), tt.icon)
		}
	}
}

func TestLoadPlugin(t *testing.T) {
	// Create a temporary plugin directory
	tmpDir := t.TempDir()

	// Create .gen-plugin/plugin.json
	pluginMetaDir := filepath.Join(tmpDir, ".gen-plugin")
	if err := os.MkdirAll(pluginMetaDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
	}
	manifestJSON, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginMetaDir, "plugin.json"), manifestJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Create skills directory with a skill
	skillsDir := filepath.Join(tmpDir, "skills", "hello")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---
name: hello
description: A greeting skill
---
Say hello!
`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create agents directory
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	agentContent := `---
name: test-agent
description: A test agent
---
You are a test agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load the plugin
	plugin, err := LoadPlugin(tmpDir, ScopeUser, "test-plugin@test")
	if err != nil {
		t.Fatalf("LoadPlugin() error = %v", err)
	}

	// Verify manifest
	if plugin.Manifest.Name != "test-plugin" {
		t.Errorf("Plugin name = %q, want %q", plugin.Manifest.Name, "test-plugin")
	}
	if plugin.Manifest.Version != "1.0.0" {
		t.Errorf("Plugin version = %q, want %q", plugin.Manifest.Version, "1.0.0")
	}

	// Verify scope and source
	if plugin.Scope != ScopeUser {
		t.Errorf("Plugin scope = %v, want %v", plugin.Scope, ScopeUser)
	}
	if plugin.Source != "test-plugin@test" {
		t.Errorf("Plugin source = %q, want %q", plugin.Source, "test-plugin@test")
	}

	// Verify components were resolved
	if len(plugin.Components.Skills) != 1 {
		t.Errorf("Plugin skills count = %d, want 1", len(plugin.Components.Skills))
	}
	if len(plugin.Components.Agents) != 1 {
		t.Errorf("Plugin agents count = %d, want 1", len(plugin.Components.Agents))
	}
}

func TestRegistry(t *testing.T) {
	// Create a test plugin
	tmpDir := t.TempDir()

	pluginMetaDir := filepath.Join(tmpDir, ".gen-plugin")
	if err := os.MkdirAll(pluginMetaDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := Manifest{Name: "registry-test"}
	manifestJSON, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginMetaDir, "plugin.json"), manifestJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Create and register plugin
	registry := NewRegistry()
	plugin, _ := LoadPlugin(tmpDir, ScopeUser, "registry-test")
	plugin.Enabled = true
	registry.Register(plugin)

	// Test Get
	got, ok := registry.Get("registry-test")
	if !ok {
		t.Error("Registry.Get() did not find plugin")
	}
	if got.Name() != "registry-test" {
		t.Errorf("Registry.Get() name = %q, want %q", got.Name(), "registry-test")
	}

	// Test List
	list := registry.List()
	if len(list) != 1 {
		t.Errorf("Registry.List() length = %d, want 1", len(list))
	}

	// Test GetEnabled
	enabled := registry.GetEnabled()
	if len(enabled) != 1 {
		t.Errorf("Registry.GetEnabled() length = %d, want 1", len(enabled))
	}

	// Test Count
	if registry.Count() != 1 {
		t.Errorf("Registry.Count() = %d, want 1", registry.Count())
	}

	// Test EnabledCount
	if registry.EnabledCount() != 1 {
		t.Errorf("Registry.EnabledCount() = %d, want 1", registry.EnabledCount())
	}

	// Test Unregister
	registry.Unregister("registry-test")
	if registry.Count() != 0 {
		t.Errorf("After Unregister, Count() = %d, want 0", registry.Count())
	}
}

func TestValidatePlugin(t *testing.T) {
	// Valid plugin
	tmpDir := t.TempDir()
	pluginMetaDir := filepath.Join(tmpDir, ".gen-plugin")
	os.MkdirAll(pluginMetaDir, 0755)

	manifest := Manifest{Name: "valid-plugin", Version: "1.0.0"}
	manifestJSON, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(pluginMetaDir, "plugin.json"), manifestJSON, 0644)

	if err := ValidatePlugin(tmpDir); err != nil {
		t.Errorf("ValidatePlugin() unexpected error = %v", err)
	}

	// Invalid plugin (no manifest)
	emptyDir := t.TempDir()
	if err := ValidatePlugin(emptyDir); err == nil {
		t.Error("ValidatePlugin() expected error for missing manifest")
	}

	// Invalid plugin (no name)
	noNameDir := t.TempDir()
	noNameMetaDir := filepath.Join(noNameDir, ".gen-plugin")
	os.MkdirAll(noNameMetaDir, 0755)
	noNameManifest := Manifest{Version: "1.0.0"} // Missing name
	noNameJSON, _ := json.Marshal(noNameManifest)
	os.WriteFile(filepath.Join(noNameMetaDir, "plugin.json"), noNameJSON, 0644)

	if err := ValidatePlugin(noNameDir); err == nil {
		t.Error("ValidatePlugin() expected error for missing name")
	}
}

func TestLoadFromPath(t *testing.T) {
	// Create a test plugin
	tmpDir := t.TempDir()

	pluginMetaDir := filepath.Join(tmpDir, ".gen-plugin")
	os.MkdirAll(pluginMetaDir, 0755)

	manifest := Manifest{Name: "path-test"}
	manifestJSON, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(pluginMetaDir, "plugin.json"), manifestJSON, 0644)

	// Test LoadFromPath
	registry := NewRegistry()
	ctx := context.Background()

	if err := registry.LoadFromPath(ctx, tmpDir); err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}

	// Verify plugin was loaded
	if registry.Count() != 1 {
		t.Errorf("After LoadFromPath, Count() = %d, want 1", registry.Count())
	}

	// Plugins loaded via path should be enabled
	got, _ := registry.Get("path-test")
	if !got.Enabled {
		t.Error("Plugin loaded via path should be enabled")
	}
}

func TestHooksConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()

	hooksDir := filepath.Join(tmpDir, "hooks")
	os.MkdirAll(hooksDir, 0755)

	hooksJSON := `{
		"hooks": {
			"PostToolUse": [
				{
					"matcher": "Write|Edit",
					"hooks": [
						{
							"type": "command",
							"command": "${GEN_PLUGIN_ROOT}/scripts/format.sh",
							"async": true
						}
					]
				}
			]
		}
	}`
	os.WriteFile(filepath.Join(hooksDir, "hooks.json"), []byte(hooksJSON), 0644)

	// Resolve hooks config
	config := ResolveHooksConfig(nil, tmpDir)
	if config == nil {
		t.Fatal("ResolveHooksConfig() returned nil")
	}

	postToolUse, ok := config.Hooks["PostToolUse"]
	if !ok {
		t.Fatal("Missing PostToolUse hooks")
	}
	if len(postToolUse) != 1 {
		t.Errorf("PostToolUse hooks length = %d, want 1", len(postToolUse))
	}

	matcher := postToolUse[0]
	if matcher.Matcher != "Write|Edit" {
		t.Errorf("Matcher = %q, want %q", matcher.Matcher, "Write|Edit")
	}
	if len(matcher.Hooks) != 1 {
		t.Errorf("Matcher hooks length = %d, want 1", len(matcher.Hooks))
	}

	cmd := matcher.Hooks[0]
	expectedCmd := tmpDir + "/scripts/format.sh"
	if cmd.Command != expectedCmd {
		t.Errorf("Hook command = %q, want %q", cmd.Command, expectedCmd)
	}
	if !cmd.Async {
		t.Error("Hook async should be true")
	}
}

func TestMCPConfigParsing(t *testing.T) {
	tmpDir := t.TempDir()

	mcpJSON := `{
		"mcpServers": {
			"database": {
				"command": "${GEN_PLUGIN_ROOT}/servers/db",
				"args": ["--config", "${GEN_PLUGIN_ROOT}/config.json"],
				"env": {
					"DB_PATH": "${GEN_PLUGIN_ROOT}/data"
				}
			}
		}
	}`
	os.WriteFile(filepath.Join(tmpDir, ".mcp.json"), []byte(mcpJSON), 0644)

	// Resolve MCP config
	servers := ResolveMCPServers(nil, tmpDir)
	if servers == nil {
		t.Fatal("ResolveMCPServers() returned nil")
	}

	db, ok := servers["database"]
	if !ok {
		t.Fatal("Missing database server")
	}

	expectedCmd := tmpDir + "/servers/db"
	if db.Command != expectedCmd {
		t.Errorf("Server command = %q, want %q", db.Command, expectedCmd)
	}

	if len(db.Args) != 2 {
		t.Fatalf("Server args length = %d, want 2", len(db.Args))
	}
	expectedArg := tmpDir + "/config.json"
	if db.Args[1] != expectedArg {
		t.Errorf("Server arg[1] = %q, want %q", db.Args[1], expectedArg)
	}

	expectedEnv := tmpDir + "/data"
	if db.Env["DB_PATH"] != expectedEnv {
		t.Errorf("Server env DB_PATH = %q, want %q", db.Env["DB_PATH"], expectedEnv)
	}
}

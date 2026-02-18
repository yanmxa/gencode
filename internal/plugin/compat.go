package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// EnvLoadClaudePlugins controls whether to load Claude Code plugins.
	EnvLoadClaudePlugins = "GEN_LOAD_CLAUDE_PLUGINS"
)

// LoadClaudePlugins loads plugins from Claude Code's plugin directories.
// This is controlled by the GEN_LOAD_CLAUDE_PLUGINS environment variable.
func (r *Registry) LoadClaudePlugins(ctx context.Context) error {
	if os.Getenv(EnvLoadClaudePlugins) != "true" {
		return nil
	}

	homeDir, _ := os.UserHomeDir()

	// Claude Code plugin locations
	claudeDirs := []string{
		filepath.Join(homeDir, ".claude", "plugins", "cache"),
		filepath.Join(homeDir, ".claude", "plugins"),
	}

	// Load enabled plugins from Claude settings
	claudeEnabled := loadClaudeEnabledPlugins(homeDir)

	for _, dir := range claudeDirs {
		plugins, err := LoadPluginsFromDir(dir, ScopeUser, "claude")
		if err != nil {
			continue
		}
		for _, p := range plugins {
			// Convert Claude plugin to GenCode format
			convertClaudePlugin(p)

			key := p.FullName()
			// Check if enabled in Claude settings
			if enabled, ok := claudeEnabled[key]; ok {
				p.Enabled = enabled
			} else if enabled, ok := claudeEnabled[p.Name()]; ok {
				p.Enabled = enabled
			} else {
				p.Enabled = true // Default enabled if in cache
			}

			r.mu.Lock()
			r.plugins[key] = p
			r.mu.Unlock()
		}
	}

	return nil
}

// loadClaudeEnabledPlugins loads enabled plugin settings from Claude Code settings.
func loadClaudeEnabledPlugins(homeDir string) map[string]bool {
	result := make(map[string]bool)

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return result
	}

	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return result
	}

	return settings.EnabledPlugins
}

// convertClaudePlugin sets the source suffix for Claude Code plugins.
func convertClaudePlugin(p *Plugin) {
	if p.Source == "" {
		p.Source = p.Name() + "@claude"
	}
}

// GetClaudePluginDirs returns Claude Code plugin directories.
func GetClaudePluginDirs() []string {
	homeDir, _ := os.UserHomeDir()
	return []string{
		filepath.Join(homeDir, ".claude", "plugins", "cache"),
		filepath.Join(homeDir, ".claude", "plugins"),
	}
}

// IsClaudePluginLoadingEnabled returns whether Claude plugin loading is enabled.
func IsClaudePluginLoadingEnabled() bool {
	return os.Getenv(EnvLoadClaudePlugins) == "true"
}

// GetClaudeInstalledPlugins reads Claude Code's installed_plugins.json.
func GetClaudeInstalledPlugins() ([]InstalledPlugin, error) {
	homeDir, _ := os.UserHomeDir()
	installedFile := filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")

	data, err := os.ReadFile(installedFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Try v2 format first
	var v2 InstalledPluginsV2
	if err := json.Unmarshal(data, &v2); err == nil && v2.Version == 2 {
		var installed []InstalledPlugin
		for source, installs := range v2.Plugins {
			if len(installs) == 0 {
				continue
			}
			info := installs[0]
			name, _ := ParsePluginRef(source)
			installed = append(installed, InstalledPlugin{
				Name:        name,
				Source:      source,
				Path:        info.InstallPath,
				Version:     info.Version,
				InstalledAt: info.InstalledAt,
			})
		}
		return installed, nil
	}

	// Fall back to v1 format
	var installed []InstalledPlugin
	if err := json.Unmarshal(data, &installed); err != nil {
		return nil, err
	}
	return installed, nil
}

// ConvertClaudeManifest converts a Claude Code manifest to GenCode format.
// The formats are largely compatible, but this handles any edge cases.
func ConvertClaudeManifest(claudeManifest map[string]any) *Manifest {
	manifest := &Manifest{
		Author:     AuthorFromAny(claudeManifest["author"]),
		Commands:   claudeManifest["commands"],
		Agents:     claudeManifest["agents"],
		Skills:     claudeManifest["skills"],
		Hooks:      claudeManifest["hooks"],
		MCPServers: claudeManifest["mcpServers"],
		LSPServers: claudeManifest["lspServers"],
	}

	// Extract string fields
	stringFields := map[string]*string{
		"name":        &manifest.Name,
		"version":     &manifest.Version,
		"description": &manifest.Description,
		"homepage":    &manifest.Homepage,
		"repository":  &manifest.Repository,
		"license":     &manifest.License,
	}
	for key, target := range stringFields {
		if v, ok := claudeManifest[key].(string); ok {
			*target = v
		}
	}

	// Extract keywords
	if keywords, ok := claudeManifest["keywords"].([]any); ok {
		for _, k := range keywords {
			if s, ok := k.(string); ok {
				manifest.Keywords = append(manifest.Keywords, s)
			}
		}
	}

	return manifest
}

// SyncFromClaudeSettings copies enabled plugin state from Claude Code settings.
func SyncFromClaudeSettings(r *Registry) error {
	homeDir, _ := os.UserHomeDir()
	enabled := loadClaudeEnabledPlugins(homeDir)

	r.mu.Lock()
	defer r.mu.Unlock()

	for name, isEnabled := range enabled {
		if p, ok := r.plugins[name]; ok {
			p.Enabled = isEnabled
		}
		// Also try with @claude suffix
		claudeName := name + "@claude"
		if p, ok := r.plugins[claudeName]; ok {
			p.Enabled = isEnabled
		}
	}

	return nil
}

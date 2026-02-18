package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Registry manages all loaded plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin // key: plugin full name (e.g., "git@marketplace")
	cwd     string             // Current working directory
	loaded  bool               // Whether plugins have been loaded
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Plugin),
	}
}

// Load loads all plugins from user, project, and local scopes.
func (r *Registry) Load(ctx context.Context, cwd string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cwd = cwd
	r.plugins = make(map[string]*Plugin)

	// Load enabled plugins from settings
	enabledPlugins, err := r.loadEnabledPlugins(cwd)
	if err != nil {
		// Non-fatal error, continue with empty enabled list
		enabledPlugins = make(map[string]bool)
	}

	// Load plugins from each scope
	for scope, dirs := range GetPluginDirs(cwd) {
		for _, dir := range dirs {
			plugins, err := LoadPluginsFromDir(dir, scope, "")
			if err != nil {
				continue
			}
			for _, p := range plugins {
				key := p.FullName()
				// Check if enabled
				if enabled, ok := enabledPlugins[key]; ok {
					p.Enabled = enabled
				} else {
					// Default: user plugins enabled, others disabled
					p.Enabled = scope == ScopeUser
				}
				r.plugins[key] = p
			}
		}
	}

	// Load from installed_plugins.json files
	for _, scope := range []Scope{ScopeUser, ScopeProject, ScopeLocal} {
		installedFile := GetInstalledPluginsFile(cwd, scope)
		plugins, err := LoadInstalledPlugins(installedFile, scope)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			key := p.FullName()
			if enabled, ok := enabledPlugins[key]; ok {
				p.Enabled = enabled
			} else {
				p.Enabled = true // Installed plugins default to enabled
			}
			r.plugins[key] = p
		}
	}

	r.loaded = true
	return nil
}

// loadEnabledPlugins loads enabled plugin settings from all config sources.
func (r *Registry) loadEnabledPlugins(cwd string) (map[string]bool, error) {
	result := make(map[string]bool)
	homeDir, _ := os.UserHomeDir()

	// Load from each settings file in priority order
	settingsFiles := []string{
		filepath.Join(homeDir, ".claude", "settings.json"),
		filepath.Join(homeDir, ".gen", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.json"),
		filepath.Join(cwd, ".gen", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.local.json"),
		filepath.Join(cwd, ".gen", "settings.local.json"),
	}

	for _, f := range settingsFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var settings struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			continue
		}
		for k, v := range settings.EnabledPlugins {
			result[k] = v
		}
	}

	return result, nil
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try exact match first
	if p, ok := r.plugins[name]; ok {
		return p, true
	}

	// Try partial match (name without marketplace)
	for key, p := range r.plugins {
		if p.Name() == name {
			return p, true
		}
		// Match the prefix before @
		if idx := len(name); idx < len(key) && key[:idx] == name && key[idx] == '@' {
			return p, true
		}
	}

	return nil, false
}

// List returns all plugins sorted by name.
func (r *Registry) List() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].FullName() < plugins[j].FullName()
	})

	return plugins
}

// GetEnabled returns all enabled plugins.
func (r *Registry) GetEnabled() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var enabled []*Plugin
	for _, p := range r.plugins {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}

	sort.Slice(enabled, func(i, j int) bool {
		return enabled[i].FullName() < enabled[j].FullName()
	})

	return enabled
}

// Enable enables a plugin.
func (r *Registry) Enable(name string, scope Scope) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}

	p.Enabled = true
	return r.saveEnabledState(name, true, scope)
}

// Disable disables a plugin.
func (r *Registry) Disable(name string, scope Scope) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin not found: %s", name)
	}

	p.Enabled = false
	return r.saveEnabledState(name, false, scope)
}

// saveEnabledState persists plugin enabled state to settings.
func (r *Registry) saveEnabledState(name string, enabled bool, scope Scope) error {
	homeDir, _ := os.UserHomeDir()

	var settingsPath string
	switch scope {
	case ScopeUser:
		settingsPath = filepath.Join(homeDir, ".gen", "settings.json")
	case ScopeProject:
		settingsPath = filepath.Join(r.cwd, ".gen", "settings.json")
	case ScopeLocal:
		settingsPath = filepath.Join(r.cwd, ".gen", "settings.local.json")
	default:
		settingsPath = filepath.Join(homeDir, ".gen", "settings.json")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	// Load existing settings
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	// Update enabled plugins
	enabledPlugins, _ := settings["enabledPlugins"].(map[string]any)
	if enabledPlugins == nil {
		enabledPlugins = make(map[string]any)
	}
	enabledPlugins[name] = enabled
	settings["enabledPlugins"] = enabledPlugins

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, data, 0644)
}

// Register adds a plugin to the registry.
func (r *Registry) Register(plugin *Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins[plugin.FullName()] = plugin
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, name)
}

// LoadFromPath loads a plugin from a specific path (for --plugin-dir).
func (r *Registry) LoadFromPath(ctx context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, err := LoadPlugin(path, ScopeLocal, "")
	if err != nil {
		return err
	}

	plugin.Enabled = true // Always enable plugins loaded via --plugin-dir
	r.plugins[plugin.FullName()] = plugin

	return nil
}

// Count returns the number of loaded plugins.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// EnabledCount returns the number of enabled plugins.
func (r *Registry) EnabledCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, p := range r.plugins {
		if p.Enabled {
			count++
		}
	}
	return count
}

// GetByScope returns plugins filtered by scope.
func (r *Registry) GetByScope(scope Scope) []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Plugin
	for _, p := range r.plugins {
		if p.Scope == scope {
			result = append(result, p)
		}
	}
	return result
}

// GetAllMCPServers returns all MCP server configs from enabled plugins.
func (r *Registry) GetAllMCPServers() map[string]MCPServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]MCPServerConfig)
	for _, p := range r.plugins {
		if !p.Enabled || p.Components.MCP == nil {
			continue
		}
		for name, config := range p.Components.MCP {
			// Namespace the server name
			key := p.Name() + ":" + name
			result[key] = config
		}
	}
	return result
}

// GetAllLSPServers returns all LSP server configs from enabled plugins.
func (r *Registry) GetAllLSPServers() map[string]LSPServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]LSPServerConfig)
	for _, p := range r.plugins {
		if !p.Enabled || p.Components.LSP == nil {
			continue
		}
		for name, config := range p.Components.LSP {
			result[name] = config
		}
	}
	return result
}

// GetAllHooks returns all hooks from enabled plugins.
func (r *Registry) GetAllHooks() map[string][]HookMatcher {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]HookMatcher)
	for _, p := range r.plugins {
		if !p.Enabled || p.Components.Hooks == nil {
			continue
		}
		for event, matchers := range p.Components.Hooks.Hooks {
			result[event] = append(result[event], matchers...)
		}
	}
	return result
}

// DefaultRegistry is the global plugin registry.
var DefaultRegistry = NewRegistry()

// Load loads plugins into the default registry.
func Load(ctx context.Context, cwd string) error {
	return DefaultRegistry.Load(ctx, cwd)
}

// LoadFromDir loads a single plugin from path into the default registry.
func LoadFromDir(ctx context.Context, path string) error {
	return DefaultRegistry.LoadFromPath(ctx, path)
}

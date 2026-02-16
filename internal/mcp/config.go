package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConfigLoader handles loading MCP configuration from multiple sources
type ConfigLoader struct {
	userDir    string // ~/.gen
	projectDir string // ./.gen or cwd
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(cwd string) *ConfigLoader {
	homeDir, _ := os.UserHomeDir()
	return &ConfigLoader{
		userDir:    filepath.Join(homeDir, ".gen"),
		projectDir: filepath.Join(cwd, ".gen"),
	}
}

// NewConfigLoaderForTest creates a configuration loader where both userDir
// and projectDir point to subdirectories of the given base directory.
// This avoids touching the real home directory in tests.
func NewConfigLoaderForTest(baseDir string) *ConfigLoader {
	return &ConfigLoader{
		userDir:    filepath.Join(baseDir, "user", ".gen"),
		projectDir: filepath.Join(baseDir, "project", ".gen"),
	}
}

// LoadAll loads and merges MCP configurations from all sources.
// Priority (lowest to highest):
//  1. ~/.gen/mcp.json (user scope)
//  2. ./.gen/mcp.json (project scope)
//  3. ./.gen/mcp.local.json (local scope)
func (l *ConfigLoader) LoadAll() (map[string]ServerConfig, error) {
	servers := make(map[string]ServerConfig)

	// Load in priority order (later overrides earlier)
	sources := []struct {
		path  string
		scope Scope
	}{
		{filepath.Join(l.userDir, "mcp.json"), ScopeUser},
		{filepath.Join(l.projectDir, "mcp.json"), ScopeProject},
		{filepath.Join(l.projectDir, "mcp.local.json"), ScopeLocal},
	}

	for _, src := range sources {
		if configs, err := l.loadFile(src.path); err == nil {
			for name, config := range configs {
				config.Name = name
				config.Scope = src.scope
				servers[name] = config
			}
		}
	}

	return servers, nil
}

// loadFile loads MCP configuration from a single file
func (l *ConfigLoader) loadFile(path string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config.MCPServers, nil
}

// GetFilePath returns the file path for a given scope
func (l *ConfigLoader) GetFilePath(scope Scope) string {
	switch scope {
	case ScopeUser:
		return filepath.Join(l.userDir, "mcp.json")
	case ScopeProject:
		return filepath.Join(l.projectDir, "mcp.json")
	case ScopeLocal:
		return filepath.Join(l.projectDir, "mcp.local.json")
	default:
		return filepath.Join(l.projectDir, "mcp.local.json")
	}
}

// SaveServer saves a server configuration to the specified scope
func (l *ConfigLoader) SaveServer(name string, config ServerConfig, scope Scope) error {
	filePath := l.GetFilePath(scope)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Load existing config
	var mcpConfig MCPConfig
	if data, err := os.ReadFile(filePath); err == nil {
		json.Unmarshal(data, &mcpConfig)
	}

	if mcpConfig.MCPServers == nil {
		mcpConfig.MCPServers = make(map[string]ServerConfig)
	}

	// Clear scope and name before saving (they're metadata, not config)
	configToSave := config
	configToSave.Name = ""
	configToSave.Scope = ""

	mcpConfig.MCPServers[name] = configToSave

	// Write back
	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// RemoveServer removes a server configuration from the specified scope
func (l *ConfigLoader) RemoveServer(name string, scope Scope) error {
	filePath := l.GetFilePath(scope)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var mcpConfig MCPConfig
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		return err
	}

	if mcpConfig.MCPServers == nil {
		return nil
	}

	delete(mcpConfig.MCPServers, name)

	// Write back
	data, err = json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// RemoveServerFromAll removes a server from all config files where it exists
func (l *ConfigLoader) RemoveServerFromAll(name string) error {
	for _, scope := range []Scope{ScopeUser, ScopeProject, ScopeLocal} {
		l.removeServerFromFile(l.GetFilePath(scope), name)
	}
	return nil
}

// removeServerFromFile removes a server from a single config file
func (l *ConfigLoader) removeServerFromFile(filePath, name string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	var mcpConfig MCPConfig
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		return
	}

	if _, exists := mcpConfig.MCPServers[name]; !exists {
		return
	}

	delete(mcpConfig.MCPServers, name)

	data, err = json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filePath, data, 0644)
}

// GetUserDir returns the user config directory
func (l *ConfigLoader) GetUserDir() string {
	return l.userDir
}

// GetProjectDir returns the project config directory
func (l *ConfigLoader) GetProjectDir() string {
	return l.projectDir
}

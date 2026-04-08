// Package plugin provides integration helpers for loading plugin components
// into the skill, agent, hooks, and MCP registries.
package plugin

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/env"
)

// InitPlugins initializes the plugin system and loads all enabled plugins.
// This should be called early in the application startup, before loading
// skills, agents, hooks, and MCP servers.
func InitPlugins(ctx context.Context, cwd string) error {
	// Load plugins from standard directories
	if err := DefaultRegistry.Load(ctx, cwd); err != nil {
		return err
	}

	// Load Claude Code plugins if enabled (non-fatal: missing plugins are skipped)
	_ = DefaultRegistry.LoadClaudePlugins(ctx)

	// Load plugins from --plugin-dir if specified
	if pluginDir := os.Getenv("GEN_PLUGIN_DIR"); pluginDir != "" {
		if err := DefaultRegistry.LoadFromPath(ctx, pluginDir); err != nil {
			return err
		}
	}

	return nil
}

// GetPluginSkillPaths returns all skill directory paths from enabled plugins.
func GetPluginSkillPaths() []PluginPath {
	return collectPluginPaths(func(p *Plugin) []string { return p.Components.Skills })
}

// GetPluginAgentPaths returns all agent file paths from enabled plugins.
func GetPluginAgentPaths() []PluginPath {
	return collectPluginPaths(func(p *Plugin) []string { return p.Components.Agents })
}

// GetPluginCommandPaths returns all command file paths from enabled plugins.
func GetPluginCommandPaths() []PluginPath {
	return collectPluginPaths(func(p *Plugin) []string { return p.Components.Commands })
}

// collectPluginPaths collects paths from enabled plugins using a getter function.
func collectPluginPaths(getPaths func(*Plugin) []string) []PluginPath {
	var paths []PluginPath
	for _, p := range DefaultRegistry.GetEnabled() {
		for _, path := range getPaths(p) {
			paths = append(paths, PluginPath{
				Path:      path,
				Namespace: p.Name(),
				Scope:     p.Scope,
			})
		}
	}
	return paths
}

// PluginPath represents a path with plugin metadata.
type PluginPath struct {
	Path      string
	Namespace string // Plugin name, used as default namespace
	Scope     Scope
}

// GetPluginHooks returns all hooks from enabled plugins in config.Hook format.
// This can be merged with the application's settings.Hooks.
func GetPluginHooks() map[string][]config.Hook {
	result := make(map[string][]config.Hook)

	for _, p := range DefaultRegistry.GetEnabled() {
		if p.Components.Hooks == nil {
			continue
		}
		for event, matchers := range p.Components.Hooks.Hooks {
			for _, matcher := range matchers {
				hook := config.Hook{
					Matcher: matcher.Matcher,
					Hooks:   make([]config.HookCmd, len(matcher.Hooks)),
				}
				for i, h := range matcher.Hooks {
					hook.Hooks[i] = config.HookCmd{
						Type:           h.Type,
						Command:        h.Command,
						Prompt:         h.Prompt,
						URL:            h.URL,
						If:             h.If,
						Shell:          h.Shell,
						Model:          h.Model,
						Async:          h.Async,
						AsyncRewake:    h.AsyncRewake,
						Timeout:        h.Timeout,
						StatusMessage:  h.StatusMessage,
						Once:           h.Once,
						Headers:        h.Headers,
						AllowedEnvVars: h.AllowedEnvVars,
					}
				}
				result[event] = append(result[event], hook)
			}
		}
	}

	return result
}

// MergePluginHooksIntoSettings merges plugin hooks into application settings.
// Plugin hooks are appended after the existing hooks for each event.
func MergePluginHooksIntoSettings(settings *config.Settings) {
	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]config.Hook)
	}

	pluginHooks := GetPluginHooks()
	for event, hooks := range pluginHooks {
		settings.Hooks[event] = append(settings.Hooks[event], hooks...)
	}
}

// PluginMCPServer represents an MCP server from a plugin with full metadata.
type PluginMCPServer struct {
	Name   string          // Full name with namespace (e.g., "plugin:server")
	Config MCPServerConfig // Server configuration
	Scope  Scope           // Plugin scope
}

// GetPluginMCPServers returns all MCP servers from enabled plugins.
func GetPluginMCPServers() []PluginMCPServer {
	var servers []PluginMCPServer
	for _, p := range DefaultRegistry.GetEnabled() {
		for name, cfg := range p.Components.MCP {
			servers = append(servers, PluginMCPServer{
				Name:   p.Name() + ":" + name,
				Config: cfg,
				Scope:  p.Scope,
			})
		}
	}
	return servers
}

// PluginLSPServer represents an LSP server from a plugin with full metadata.
type PluginLSPServer struct {
	Name   string          // Language name (e.g., "go", "rust")
	Config LSPServerConfig // Server configuration
	Scope  Scope           // Plugin scope
}

// GetPluginLSPServers returns all LSP servers from enabled plugins.
func GetPluginLSPServers() []PluginLSPServer {
	var servers []PluginLSPServer
	for _, p := range DefaultRegistry.GetEnabled() {
		for name, cfg := range p.Components.LSP {
			servers = append(servers, PluginLSPServer{
				Name:   name,
				Config: cfg,
				Scope:  p.Scope,
			})
		}
	}
	return servers
}

// GetPluginNamespace extracts the namespace from a plugin path or source.
func GetPluginNamespace(source string) string {
	name, _ := ParsePluginRef(source)
	return name
}

// activeRoot tracks the plugin root for the currently executing skill or command.
// Protected by activeRootMu for concurrent safety.
var (
	activeRootMu sync.RWMutex
	activeRoot   string
)

// SetActivePluginRoot sets the root path for the currently executing plugin.
// Called when a plugin skill or custom command is invoked.
func SetActivePluginRoot(path string) {
	activeRootMu.Lock()
	activeRoot = path
	activeRootMu.Unlock()
}

// ClearActivePluginRoot clears the active plugin root.
// Called when the user submits a new regular message.
func ClearActivePluginRoot() {
	activeRootMu.Lock()
	activeRoot = ""
	activeRootMu.Unlock()
}

// getActivePluginRoot returns the current active plugin root.
func getActivePluginRoot() string {
	activeRootMu.RLock()
	defer activeRootMu.RUnlock()
	return activeRoot
}

// FindPluginRootForPath returns the plugin root that contains the given path,
// or "" if no enabled plugin matches.
func FindPluginRootForPath(path string) string {
	if DefaultRegistry == nil || path == "" {
		return ""
	}
	for _, p := range DefaultRegistry.GetEnabled() {
		if strings.HasPrefix(path, p.Path+"/") || path == p.Path {
			return p.Path
		}
	}
	return ""
}

// PluginEnv returns environment variables for all enabled plugins.
// Callers should append the result to os.Environ() when spawning child
// processes so that plugin scripts can locate their resources via
// GEN_PLUGIN_ROOT / CLAUDE_PLUGIN_ROOT.
//
// Per plugin:
//
//	GEN_PLUGIN_ROOT_<UPPER_NAME>=<path>   CLAUDE_PLUGIN_ROOT_<UPPER_NAME>=<path>
//
// Unqualified alias — points to the active plugin root if set, otherwise
// falls back to the sole enabled plugin (if exactly one):
//
//	GEN_PLUGIN_ROOT=<path>   CLAUDE_PLUGIN_ROOT=<path>
func PluginEnv() []string {
	if DefaultRegistry == nil {
		return nil
	}
	enabled := DefaultRegistry.GetEnabled()
	if len(enabled) == 0 {
		return nil
	}

	var out []string
	for _, p := range enabled {
		out = append(out, env.PairF("PLUGIN_ROOT_%s", envSafeName(p.Name()), p.Path)...)
	}

	root := getActivePluginRoot()
	if root == "" && len(enabled) == 1 {
		root = enabled[0].Path
	}
	if root != "" {
		out = append(out, env.Pair("PLUGIN_ROOT", root)...)
	}
	return out
}

// envSafeName converts a plugin name to an environment-variable-safe
// upper-case identifier: lowercase, hyphens/dots → underscores.
func envSafeName(name string) string {
	s := strings.ToUpper(name)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

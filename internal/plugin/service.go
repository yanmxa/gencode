package plugin

import (
	"sync"

	"github.com/yanmxa/gencode/internal/setting"
)

// Service is the public contract for the plugin module.
type Service interface {
	// query
	List() []*Plugin                     // all loaded plugins
	Get(name string) (*Plugin, bool)     // lookup by name
	GetEnabled() []*Plugin               // all enabled plugins
	Count() int                          // total number of loaded plugins
	EnabledCount() int                   // number of enabled plugins

	// mutation
	Enable(name string, scope Scope) error
	Disable(name string, scope Scope) error

	// cross-domain (consumed by other services at init)
	AgentPaths() []PluginPath            // agent file paths from enabled plugins
	SkillPaths() []PluginPath            // skill directory paths from enabled plugins
	CommandPaths() []PluginPath          // command file paths from enabled plugins
	MCPServers() []PluginMCPServer       // MCP servers from enabled plugins
	PluginHooks() map[string][]setting.Hook // hook definitions from enabled plugins
	PluginEnv() []string                 // environment variables for enabled plugins
}

// Compile-time check: *service implements Service.
var _ Service = (*service)(nil)

// ── singleton ──────────────────────────────────────────────

var (
	svcMu      sync.RWMutex
	svcInstance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := svcInstance
	svcMu.RUnlock()
	if s == nil {
		panic("plugin: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	svcInstance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	svcInstance = nil
	svcMu.Unlock()
}

// ── implementation ─────────────────────────────────────────

// service wraps the legacy Registry to satisfy the Service interface.
type service struct {
	registry *Registry
}

func (s *service) List() []*Plugin                { return s.registry.List() }
func (s *service) Get(name string) (*Plugin, bool) { return s.registry.Get(name) }
func (s *service) GetEnabled() []*Plugin           { return s.registry.GetEnabled() }
func (s *service) Count() int                      { return s.registry.Count() }
func (s *service) EnabledCount() int               { return s.registry.EnabledCount() }

func (s *service) Enable(name string, scope Scope) error  { return s.registry.Enable(name, scope) }
func (s *service) Disable(name string, scope Scope) error { return s.registry.Disable(name, scope) }

func (s *service) AgentPaths() []PluginPath   { return GetPluginAgentPaths() }
func (s *service) SkillPaths() []PluginPath   { return GetPluginSkillPaths() }
func (s *service) CommandPaths() []PluginPath { return GetPluginCommandPaths() }
func (s *service) MCPServers() []PluginMCPServer { return GetPluginMCPServers() }
func (s *service) PluginHooks() map[string][]setting.Hook { return GetPluginHooks() }
func (s *service) PluginEnv() []string { return PluginEnv() }

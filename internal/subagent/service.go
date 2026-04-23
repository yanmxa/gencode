package subagent

import (
	"fmt"
	"sync"

	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
)

// Service is the public contract for the subagent module.
type Service interface {
	// query
	ListConfigs() []*AgentConfig                  // all agent type definitions
	Get(name string) (*AgentConfig, bool)         // lookup by name
	IsEnabled(name string) bool                   // check if enabled
	GetDisabledAt(userLevel bool) map[string]bool // disabled agents at level

	// mutation
	SetEnabled(name string, enabled bool, userLevel bool) error
	Register(config *AgentConfig) // add an agent configuration

	// factory
	NewExecutor(provider llm.Provider, cwd string, parentModelID string, hookEngine *hook.Engine) *Executor

	// system prompt
	PromptSection() string // rendered section for system prompt

	// concrete access
	Registry() *Registry // returns the underlying Registry for adapter construction
}

// Compile-time check: *Registry implements Service.
var _ Service = (*Registry)(nil)

// Options holds all dependencies for initialization.
type Options struct {
	CWD              string
	PluginAgentPaths func() []PluginAgentPath
}

// Initialize loads custom agents from all sources and initializes state stores.
func Initialize(opts Options) error {
	ClearPluginAgentPaths()

	if opts.PluginAgentPaths != nil {
		for _, pp := range opts.PluginAgentPaths() {
			AddPluginAgentPath(pp.Path, pp.Namespace)
		}
	}

	LoadCustomAgents(opts.CWD)

	if err := defaultRegistry.InitStores(opts.CWD); err != nil {
		return fmt.Errorf("failed to initialize agent stores: %w", err)
	}

	SetDefault(defaultRegistry)
	return nil
}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("subagent: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}

package subagent

import (
	"sync"

	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
)

// Service is the public contract for the subagent module.
type Service interface {
	// query
	ListConfigs() []*AgentConfig                          // all agent type definitions
	Get(name string) (*AgentConfig, bool)                 // lookup by name
	IsEnabled(name string) bool                           // check if enabled
	GetDisabledAt(userLevel bool) map[string]bool         // disabled agents at level

	// mutation
	SetEnabled(name string, enabled bool, userLevel bool) error
	Register(config *AgentConfig)                         // add an agent configuration

	// factory
	NewExecutor(provider llm.Provider, cwd string, parentModelID string, hookEngine *hook.Engine) *Executor

	// system prompt
	PromptSection() string // rendered section for system prompt
}

// Compile-time check: *Registry implements Service.
var _ Service = (*Registry)(nil)

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
		panic("subagent: not initialized")
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

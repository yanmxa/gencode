package tool

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// Service is the public contract for the tool module.
type Service interface {
	// registration
	Register(t Tool)                                   // add a tool
	RegisterAlias(alias string, t Tool)                // add an alias for a tool
	Get(name string) (Tool, bool)                      // lookup by name
	List() []string                                    // all registered tool names

	// execution
	Execute(ctx context.Context, name string, params map[string]any, cwd string) toolresult.ToolResult
}

// Compile-time check: *Registry implements Service.
var _ Service = (*Registry)(nil)

// ── singleton ──────────────────────────────────────────────

var (
	svcMu      sync.RWMutex
	svcInstance Service
)

// Default returns the singleton Service instance.
// Falls back to DefaultRegistry if no explicit instance has been set,
// ensuring backward compatibility with existing init()-time registrations.
func Default() Service {
	svcMu.RLock()
	s := svcInstance
	svcMu.RUnlock()
	if s != nil {
		return s
	}
	return DefaultRegistry
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

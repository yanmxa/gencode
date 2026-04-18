package tool

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// Service is the public contract for the tool module.
type Service interface {
	// registration
	Register(t Tool)
	RegisterAlias(alias string, t Tool)
	Get(name string) (Tool, bool)
	List() []string

	// execution
	Execute(ctx context.Context, name string, params map[string]any, cwd string) toolresult.ToolResult

	// deferred tools
	ResetFetched()
	FormatDeferredToolsPrompt() string

	// side effects
	PopSideEffect(toolCallID string) any
}

// Compile-time check: *Registry implements Service.
var _ Service = (*Registry)(nil)

// Options holds all dependencies for initialization.
type Options struct{}

// ── singleton ──────────────────────────────────────────────

var (
	svcMu      sync.RWMutex
	svcInstance Service
)

// Initialize sets the singleton to the DefaultRegistry.
func Initialize(opts Options) {
	svcMu.Lock()
	svcInstance = DefaultRegistry
	svcMu.Unlock()
}

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

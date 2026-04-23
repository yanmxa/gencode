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
	mu       sync.RWMutex
	instance Service
)

// Initialize sets the singleton to the default registry.
func Initialize(opts Options) {
	mu.Lock()
	instance = defaultRegistry
	mu.Unlock()
}

// Default returns the singleton Service instance.
// Falls back to defaultRegistry if Initialize has not been called,
// since tools register at init time before Initialize runs.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s != nil {
		return s
	}
	return defaultRegistry
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

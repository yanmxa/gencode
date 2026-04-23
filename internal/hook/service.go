package hook

import (
	"context"
	"sync"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/setting"
)

// Service is the public contract for the hook module.
type Service interface {
	// execution
	Execute(ctx context.Context, event EventType, input HookInput) HookOutcome
	ExecuteAsync(event EventType, input HookInput)
	FilterToolCalls(ctx context.Context, calls []core.ToolCall, agentID, agentType string) FilterToolCallsResult

	// query
	HasHooks(event EventType) bool
	StopHookActive() *bool
	CurrentStatusMessage() string

	// reconfigure (after session/provider/cwd change)
	SetSettings(settings *setting.Settings)
	SetLLMCompleter(fn LLMCompleter, model string)
	SetTranscriptPath(path string)
	SetCwd(cwd string)
	SetPermissionMode(mode string)
	SetPromptCallback(cb PromptCallback)
	SetAsyncHookCallback(cb AsyncHookCallback)
	SetEnvProvider(fn func() []string)

	// session-scoped hooks
	AddSessionFunctionHook(event EventType, matcher string, hook FunctionHook) string
	AddRuntimeFunctionHook(event EventType, matcher string, hook FunctionHook) string
	ClearSessionHooks()

	// lifecycle
	Wait()

	Engine() *Engine
}

// Engine returns the receiver itself, satisfying the Service interface.
func (e *Engine) Engine() *Engine { return e }

// Compile-time check: *Engine implements Service.
var _ Service = (*Engine)(nil)

// Options holds the dependencies needed to create the default hook engine.
// All fields must be supplied by the caller — the hook package does not reach into
// global singletons.
type Options struct {
	Settings       *setting.Settings
	SessionID      string
	CWD            string
	TranscriptPath string
	Completer      LLMCompleter
	ModelID        string
	EnvProvider    func() []string
}

// Initialize creates the singleton hook engine from the given options.
func Initialize(opts Options) {
	e := NewEngine(opts.Settings, opts.SessionID, opts.CWD, opts.TranscriptPath)
	e.SetLLMCompleter(opts.Completer, opts.ModelID)
	if opts.EnvProvider != nil {
		e.SetEnvProvider(opts.EnvProvider)
	}
	SetDefault(e)
}

// -- singleton ----------------------------------------------------------

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
		panic("hook: not initialized")
	}
	return s
}

// DefaultIfInit returns the singleton Service instance, or nil if not yet
// initialized. Used during early initialization when the hook service may
// not be ready.
func DefaultIfInit() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
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

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

	// Engine returns the underlying *Engine.
	// This is needed by callers that require the concrete type
	// (e.g. notify.InstallCompletionObserver, subagent.NewExecutor, trigger.NewFileWatcher).
	Engine() *Engine
}

// Engine returns the receiver itself, satisfying the Service interface.
func (e *Engine) Engine() *Engine { return e }

// Compile-time check: *Engine implements Service.
var _ Service = (*Engine)(nil)

// -- singleton ----------------------------------------------------------

var (
	svcMu    sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := instance
	svcMu.RUnlock()
	if s == nil {
		panic("hook: not initialized")
	}
	return s
}

// DefaultIfInit returns the singleton Service instance, or nil if not yet
// initialized. This supports callers that check `if hook.DefaultEngine != nil`
// before calling methods.
func DefaultIfInit() Service {
	svcMu.RLock()
	s := instance
	svcMu.RUnlock()
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	instance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}

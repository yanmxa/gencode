package hooks

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
)

// DefaultTimeout is the default timeout for hook commands in seconds.
const DefaultTimeout = 600

// Engine executes hooks from settings, plugins, and runtime/session registration.
type Engine struct {
	settings       *config.Settings
	sessionID      string
	cwd            string
	transcriptPath string
	permissionMode string

	promptCallback PromptCallback
	llmProvider    provider.LLMProvider
	hookModel      string
	httpClient     *http.Client
	agentRunner    AgentRunner
	asyncCallback  AsyncHookCallback

	mu            sync.RWMutex
	sessionHooks  map[EventType][]config.Hook
	runtimeHooks  map[EventType][]config.Hook
	sessionFuncs  map[EventType][]functionHookRegistration
	runtimeFuncs  map[EventType][]functionHookRegistration
	executedOnce  map[string]struct{}
	functionSeqNo atomic.Uint64
	statusSeq     atomic.Uint64
	activeStatus  map[string]activeHookStatus
}

type functionHookRegistration struct {
	Matcher string
	Hook    FunctionHook
}

type activeHookStatus struct {
	Message string
	Seq     uint64
}

// NewEngine creates a new hook execution engine.
func NewEngine(settings *config.Settings, sessionID, cwd, transcriptPath string) *Engine {
	return &Engine{
		settings:       settings,
		sessionID:      sessionID,
		cwd:            cwd,
		transcriptPath: transcriptPath,
		permissionMode: "default",
		httpClient:     http.DefaultClient,
		sessionHooks:   make(map[EventType][]config.Hook),
		runtimeHooks:   make(map[EventType][]config.Hook),
		sessionFuncs:   make(map[EventType][]functionHookRegistration),
		runtimeFuncs:   make(map[EventType][]functionHookRegistration),
		executedOnce:   make(map[string]struct{}),
		activeStatus:   make(map[string]activeHookStatus),
	}
}

// SetPermissionMode sets the current permission mode (normal, auto, plan).
func (e *Engine) SetPermissionMode(mode string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.permissionMode = mode
}

// SetPromptCallback sets the callback for bidirectional prompt exchanges.
func (e *Engine) SetPromptCallback(cb PromptCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.promptCallback = cb
}

// SetTranscriptPath updates the transcript path after engine creation.
func (e *Engine) SetTranscriptPath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transcriptPath = path
}

// SetCwd updates the working directory used for hook input and subprocess execution.
func (e *Engine) SetCwd(cwd string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cwd = cwd
}

// SetLLMProvider configures the provider/model used by prompt and agent hooks.
func (e *Engine) SetLLMProvider(p provider.LLMProvider, model string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmProvider = p
	e.hookModel = model
}

// SetHTTPClient configures the HTTP client used by http hooks.
func (e *Engine) SetHTTPClient(client *http.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if client != nil {
		e.httpClient = client
	}
}

// SetAgentRunner configures the multi-turn runner used by agent hooks.
func (e *Engine) SetAgentRunner(runner AgentRunner) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agentRunner = runner
}

// SetAsyncHookCallback configures delivery of background asyncRewake hook results.
func (e *Engine) SetAsyncHookCallback(cb AsyncHookCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.asyncCallback = cb
}

// SetSettings swaps the settings-backed hook source used by the engine.
func (e *Engine) SetSettings(settings *config.Settings) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settings = settings
}

// AddSessionHook registers an in-memory session-scoped hook.
func (e *Engine) AddSessionHook(event EventType, matcher string, hook config.HookCmd) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionHooks[event] = append(e.sessionHooks[event], config.Hook{
		Matcher: matcher,
		Hooks:   []config.HookCmd{hook},
	})
}

// ClearSessionHooks removes all session-scoped hooks.
func (e *Engine) ClearSessionHooks() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionHooks = make(map[EventType][]config.Hook)
	e.sessionFuncs = make(map[EventType][]functionHookRegistration)
}

// AddRuntimeHook registers a process-level runtime hook.
func (e *Engine) AddRuntimeHook(event EventType, matcher string, hook config.HookCmd) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runtimeHooks[event] = append(e.runtimeHooks[event], config.Hook{
		Matcher: matcher,
		Hooks:   []config.HookCmd{hook},
	})
}

// AddSessionFunctionHook registers an in-memory function hook scoped to the
// current engine/session instance.
func (e *Engine) AddSessionFunctionHook(event EventType, matcher string, hook FunctionHook) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	hook.ID = e.ensureFunctionHookIDLocked("session", event, hook.ID)
	e.sessionFuncs[event] = append(e.sessionFuncs[event], functionHookRegistration{
		Matcher: matcher,
		Hook:    hook,
	})
	return hook.ID
}

// AddRuntimeFunctionHook registers an in-memory function hook for the current
// process.
func (e *Engine) AddRuntimeFunctionHook(event EventType, matcher string, hook FunctionHook) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	hook.ID = e.ensureFunctionHookIDLocked("runtime", event, hook.ID)
	e.runtimeFuncs[event] = append(e.runtimeFuncs[event], functionHookRegistration{
		Matcher: matcher,
		Hook:    hook,
	})
	return hook.ID
}

// RemoveSessionFunctionHook removes a session-scoped function hook by ID.
func (e *Engine) RemoveSessionFunctionHook(event EventType, id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return removeFunctionHookByID(e.sessionFuncs, event, id)
}

// RemoveRuntimeFunctionHook removes a process-level function hook by ID.
func (e *Engine) RemoveRuntimeFunctionHook(event EventType, id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return removeFunctionHookByID(e.runtimeFuncs, event, id)
}

// Execute runs all matching hooks for an event synchronously.
func (e *Engine) Execute(ctx context.Context, event EventType, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}
	hooks := e.getMatchingHooks(event, &input)
	if len(hooks) == 0 {
		return outcome
	}

	for _, hook := range hooks {
		if hook.Command != nil && (hook.Command.Async || hook.Command.AsyncRewake) {
			hookCopy, inputCopy := hook, input
			go e.executeDetachedHook(context.Background(), hookCopy, inputCopy)
			continue
		}

		result := e.executeMatchedHook(ctx, hook, input)
		if result.Error != nil {
			continue
		}
		if !result.ShouldContinue {
			return result
		}
		outcome = e.mergeOutcome(outcome, result)
	}

	return outcome
}

// ExecuteAsync runs all matching hooks asynchronously (fire-and-forget).
func (e *Engine) ExecuteAsync(event EventType, input HookInput) {
	hooks := e.getMatchingHooks(event, &input)
	for _, hook := range hooks {
		hookCopy, inputCopy := hook, input
		go e.executeDetachedHook(context.Background(), hookCopy, inputCopy)
	}
}

// HasHooks returns true if there are any hooks configured for the given event.
func (e *Engine) HasHooks(event EventType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.settings != nil && len(e.settings.Hooks[string(event)]) > 0 {
		return true
	}
	return len(e.sessionHooks[event]) > 0 || len(e.runtimeHooks[event]) > 0 ||
		len(e.sessionFuncs[event]) > 0 || len(e.runtimeFuncs[event]) > 0
}

// StopHookActive returns a *bool indicating whether Stop hooks are configured.
func (e *Engine) StopHookActive() *bool {
	active := e.HasHooks(Stop)
	return &active
}

func (e *Engine) getAgentRunner() AgentRunner {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.agentRunner
}

func (e *Engine) getAsyncHookCallback() AsyncHookCallback {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.asyncCallback
}

// CurrentStatusMessage returns the most recently-started active hook status message.
func (e *Engine) CurrentStatusMessage() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var (
		message string
		maxSeq  uint64
	)
	for _, status := range e.activeStatus {
		if status.Seq >= maxSeq {
			maxSeq = status.Seq
			message = status.Message
		}
	}
	return message
}

func (e *Engine) ensureFunctionHookIDLocked(scope string, event EventType, current string) string {
	if current != "" {
		return current
	}
	seq := e.functionSeqNo.Add(1)
	return fmt.Sprintf("%s-%s-%d", scope, event, seq)
}

func removeFunctionHookByID(store map[EventType][]functionHookRegistration, event EventType, id string) bool {
	hooks := store[event]
	if len(hooks) == 0 {
		return false
	}

	filtered := hooks[:0]
	removed := false
	for _, hook := range hooks {
		if !removed && hook.Hook.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, hook)
	}

	if !removed {
		return false
	}
	if len(filtered) == 0 {
		delete(store, event)
		return true
	}
	store[event] = append([]functionHookRegistration(nil), filtered...)
	return true
}

func (e *Engine) startStatus(message string) string {
	if message == "" {
		return ""
	}
	seq := e.statusSeq.Add(1)
	key := fmt.Sprintf("status-%d", seq)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeStatus[key] = activeHookStatus{
		Message: message,
		Seq:     seq,
	}
	return key
}

func (e *Engine) endStatus(id string) {
	if id == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.activeStatus, id)
}

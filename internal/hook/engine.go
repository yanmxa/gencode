package hook

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/setting"
)

// LLMCompleter performs a single-turn LLM completion for hook execution.
// The caller supplies the system prompt, user message, and model identifier;
// the implementation owns provider construction and streaming details.
type LLMCompleter func(ctx context.Context, systemPrompt, userMessage, model string) (string, error)

// defaultTimeout is the default timeout for hook commands in seconds.
const defaultTimeout = 600

// Engine executes hooks from settings, plugins, and runtime/session registration.
type Engine struct {
	settings       *setting.Settings
	sessionID      string
	cwd            string
	transcriptPath string
	permissionMode string

	promptCallback PromptCallback
	llmCompleter   LLMCompleter
	hookModel      string
	httpClient     *http.Client
	asyncCallback  AsyncHookCallback
	envProvider    func() []string

	mu         sync.RWMutex
	store      *hookStore
	status     *statusTracker
	detachedWg sync.WaitGroup // tracks fire-and-forget goroutines
}

// DefaultEngine is the singleton hook engine, initialized by Init().
var DefaultEngine *Engine

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
	DefaultEngine = NewEngine(opts.Settings, opts.SessionID, opts.CWD, opts.TranscriptPath)
	DefaultEngine.SetLLMCompleter(opts.Completer, opts.ModelID)
	if opts.EnvProvider != nil {
		DefaultEngine.SetEnvProvider(opts.EnvProvider)
	}
	SetDefault(DefaultEngine)
}

// NewEngine creates a new hook execution engine.
func NewEngine(settings *setting.Settings, sessionID, cwd, transcriptPath string) *Engine {
	return &Engine{
		settings:       settings,
		sessionID:      sessionID,
		cwd:            cwd,
		transcriptPath: transcriptPath,
		permissionMode: "default",
		httpClient:     http.DefaultClient,
		store:          newHookStore(),
		status:         newStatusTracker(),
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

// SetLLMCompleter configures the completion function and default model used by
// prompt and agent hooks.
func (e *Engine) SetLLMCompleter(fn LLMCompleter, model string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmCompleter = fn
	e.hookModel = model
}


// SetAsyncHookCallback configures delivery of background asyncRewake hook results.
func (e *Engine) SetAsyncHookCallback(cb AsyncHookCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.asyncCallback = cb
}

// SetEnvProvider configures a function that supplies additional environment
// variables for hook subprocess execution.
func (e *Engine) SetEnvProvider(fn func() []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.envProvider = fn
}

// SetSettings swaps the settings-backed hook source used by the engine.
// Callers should merge plugin hooks into settings before calling this.
func (e *Engine) SetSettings(settings *setting.Settings) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.settings = settings
}

// ClearSessionHooks removes all session-scoped hooks.
func (e *Engine) ClearSessionHooks() {
	e.store.ClearSessionHooks()
}

// AddSessionFunctionHook registers an in-memory function hook scoped to the
// current engine/session instance.
func (e *Engine) AddSessionFunctionHook(event EventType, matcher string, hook FunctionHook) string {
	return e.store.AddSessionFunctionHook(event, matcher, hook)
}

// AddRuntimeFunctionHook registers an in-memory function hook for the current
// process.
func (e *Engine) AddRuntimeFunctionHook(event EventType, matcher string, hook FunctionHook) string {
	return e.store.AddRuntimeFunctionHook(event, matcher, hook)
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
			e.detachedWg.Add(1)
			go func() {
				defer e.detachedWg.Done()
				e.executeDetachedHook(context.Background(), hookCopy, inputCopy)
			}()
			continue
		}

		result := e.executeMatchedHook(ctx, hook, input)
		if result.Error != nil {
			log.Logger().Warn("hook execution failed",
				zap.String("event", string(event)),
				zap.String("source", result.HookSource),
				zap.Error(result.Error),
			)
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
		e.detachedWg.Add(1)
		go func() {
			defer e.detachedWg.Done()
			e.executeDetachedHook(context.Background(), hookCopy, inputCopy)
		}()
	}
}

// HasHooks returns true if there are any hooks configured for the given event.
func (e *Engine) HasHooks(event EventType) bool {
	e.mu.RLock()
	settings := e.settings
	e.mu.RUnlock()
	return e.store.HasHooks(event, settings)
}

// StopHookActive returns a *bool indicating whether Stop hooks are configured.
func (e *Engine) StopHookActive() *bool {
	active := e.HasHooks(Stop)
	return &active
}

func (e *Engine) getAsyncHookCallback() AsyncHookCallback {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.asyncCallback
}

// Wait waits for all detached async hook goroutines to finish.
func (e *Engine) Wait() {
	e.detachedWg.Wait()
}

// CurrentStatusMessage returns the most recently-started active hook status.
func (e *Engine) CurrentStatusMessage() string {
	return e.status.CurrentMessage()
}

package hook

import (
	"context"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
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
	handlers   core.Hooks
	detachedWg sync.WaitGroup // tracks fire-and-forget goroutines
}

// DefaultEngine is the singleton hook engine, initialized by Init().
var DefaultEngine *Engine

// Initialize creates the singleton hook engine from domain singletons.
// Reads settings, session, LLM, and plugin state — caller only needs to
// inject AgentRunner afterwards (it lives in the app layer).
func Initialize(cwd string) {
	settings := setting.DefaultSetup
	plugin.MergePluginHooksIntoSettings(settings)

	sessionID := session.DefaultSetup.SessionID
	transcriptPath := session.DefaultSetup.TranscriptPath()

	DefaultEngine = NewEngine(settings, sessionID, cwd, transcriptPath)

	modelID := llm.DefaultSetup.ModelID()
	DefaultEngine.SetLLMCompleter(buildLLMCompleter(llm.DefaultSetup.Provider), modelID)
	DefaultEngine.SetEnvProvider(plugin.PluginEnv)
}

// buildLLMCompleter wraps a provider into an LLMCompleter closure.
func buildLLMCompleter(p llm.Provider) LLMCompleter {
	if p == nil {
		return nil
	}
	return func(ctx context.Context, systemPrompt, userMessage, model string) (string, error) {
		c := llm.NewClient(p, model, 0)
		resp, err := c.Complete(ctx, systemPrompt, []core.Message{{
			Role:    core.RoleUser,
			Content: userMessage,
		}}, 4096)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
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
		handlers:       core.NewHooks(),
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

// SetLLMProvider configures the LLM provider and model for hook execution.
func (e *Engine) SetLLMProvider(p llm.Provider, model string) {
	e.SetLLMCompleter(buildLLMCompleter(p), model)
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
func (e *Engine) SetSettings(settings *setting.Settings) {
	plugin.MergePluginHooksIntoSettings(settings)
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

// Drain waits for all async hook goroutines to finish, including both
// core.Hooks goroutines and engine-level detached goroutines.
func (e *Engine) Drain() {
	e.handlers.Drain()
	e.detachedWg.Wait()
}

// AsCoreHooks returns the Engine as a core.Hooks interface.
// Engine directly satisfies core.Hooks via Register/Unregister/Fire/Has/Drain.
func AsCoreHooks(engine *Engine) core.Hooks {
	if engine == nil {
		return nil
	}
	return engine
}

// CurrentStatusMessage returns the most recently-started active hook status core.
func (e *Engine) CurrentStatusMessage() string {
	return e.status.CurrentMessage()
}

// Register registers a core.Hook and returns its ID.
func (e *Engine) Register(hook core.Hook) string {
	return e.handlers.Register(hook)
}

// Unregister removes a core.Hook by ID.
func (e *Engine) Unregister(id string) bool {
	return e.handlers.Unregister(id)
}

// Fire executes Go handlers first, then config-driven hooks if the event maps.
func (e *Engine) Fire(ctx context.Context, event core.Event) (core.Action, error) {
	action, err := e.handlers.Fire(ctx, event)
	if err != nil || action.Block {
		return action, err
	}

	engineEvent, ok := coreToEngineEvent(event)
	if !ok {
		return action, nil
	}

	input := buildHookInput(event)
	outcome := e.Execute(ctx, engineEvent, input)
	engineAction := outcomeToAction(outcome)

	return core.MergeActions(action, engineAction), nil
}

// Has returns true if any Go handlers or config-driven hooks exist for the event.
func (e *Engine) Has(event core.EventType) bool {
	if e.handlers.Has(event) {
		return true
	}
	switch event {
	case core.PostTool:
		return e.HasHooks(PostToolUse) || e.HasHooks(PostToolUseFailure)
	default:
		if engineEvent, ok := coreToEngineEventType(event); ok {
			return e.HasHooks(engineEvent)
		}
		return false
	}
}

var coreToEngineEventMap = map[core.EventType]core.EventType{
	core.OnStart: SessionStart,
	core.OnStop:  Stop,
	core.PreTool: PreToolUse,
}

func coreToEngineEventType(event core.EventType) (core.EventType, bool) {
	e, ok := coreToEngineEventMap[event]
	return e, ok
}

func coreToEngineEvent(event core.Event) (core.EventType, bool) {
	if event.Type == core.PostTool {
		switch tr := event.Data.(type) {
		case core.ToolResult:
			if tr.IsError {
				return PostToolUseFailure, true
			}
		}
		return PostToolUse, true
	}
	e, ok := coreToEngineEventMap[event.Type]
	return e, ok
}

func buildHookInput(event core.Event) HookInput {
	input := HookInput{
		HookEventName: string(event.Type),
	}

	switch event.Type {
	case core.PreTool:
		if tc, ok := event.Data.(core.ToolCall); ok {
			input.ToolName = tc.Name
			input.ToolUseID = tc.ID
			if params, _ := core.ParseToolInput(tc.Input); params != nil {
				input.ToolInput = params
			}
		}
	case core.PostTool:
		switch tr := event.Data.(type) {
		case core.ToolResult:
			input.ToolName = tr.ToolName
			input.ToolUseID = tr.ToolCallID
			input.ToolResponse = tr.Content
			if tr.IsError {
				input.Error = tr.Content
			}
		}
	case core.OnStop:
		if errVal, ok := event.Data.(error); ok && errVal != nil {
			input.Error = errVal.Error()
		}
	}

	return input
}

func outcomeToAction(outcome HookOutcome) core.Action {
	var action core.Action

	if outcome.ShouldBlock {
		action.Block = true
		action.Reason = outcome.BlockReason
	}

	if outcome.UpdatedInput != nil {
		action.Modify = outcome.UpdatedInput
	}

	if outcome.AdditionalContext != "" {
		action.Inject = outcome.AdditionalContext
	}

	meta := make(map[string]any)
	if len(outcome.UpdatedPermissions) > 0 {
		meta["updated_permissions"] = outcome.UpdatedPermissions
	}
	if len(outcome.WatchPaths) > 0 {
		meta["watch_paths"] = outcome.WatchPaths
	}
	if outcome.InitialUserMessage != "" {
		meta["initial_user_message"] = outcome.InitialUserMessage
	}
	if outcome.Retry {
		meta["retry"] = true
	}
	if len(meta) > 0 {
		action.Meta = meta
	}

	return action
}

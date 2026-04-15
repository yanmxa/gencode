package hooks

import (
	"context"
	"net/http"
	"sync"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
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

	mu           sync.RWMutex
	store        *HookStore
	status       *StatusTracker
	handlers     core.Hooks
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
		store:          NewHookStore(),
		status:         NewStatusTracker(),
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
	e.store.AddSessionHook(event, matcher, hook)
}

// ClearSessionHooks removes all session-scoped hooks.
func (e *Engine) ClearSessionHooks() {
	e.store.ClearSessionHooks()
}

// AddRuntimeHook registers a process-level runtime hook.
func (e *Engine) AddRuntimeHook(event EventType, matcher string, hook config.HookCmd) {
	e.store.AddRuntimeHook(event, matcher, hook)
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

// RemoveSessionFunctionHook removes a session-scoped function hook by ID.
func (e *Engine) RemoveSessionFunctionHook(event EventType, id string) bool {
	return e.store.RemoveSessionFunctionHook(event, id)
}

// RemoveRuntimeFunctionHook removes a process-level function hook by ID.
func (e *Engine) RemoveRuntimeFunctionHook(event EventType, id string) bool {
	return e.store.RemoveRuntimeFunctionHook(event, id)
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
	settings := e.settings
	e.mu.RUnlock()
	return e.store.HasHooks(event, settings)
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
		switch tc := event.Data.(type) {
		case core.ToolCall:
			input.ToolName = tc.Name
			input.ToolInput = tc.Input
			input.ToolUseID = tc.ID
		case message.ToolCall:
			input.ToolName = tc.Name
			if params, _ := message.ParseToolInput(tc.Input); params != nil {
				input.ToolInput = params
			}
			input.ToolUseID = tc.ID
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

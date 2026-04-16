package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// agent is the default Agent implementation.
type agent struct {
	id                string
	system            System
	tools             Tools
	hooks             Hooks
	permission        PermissionFunc
	llm               LLM
	cwd               string
	maxTurns          int
	maxOutputRecovery int
	inbox             chan Message
	outbox            chan Event

	mu       sync.RWMutex
	messages []Message // conversation history

	closed atomic.Bool // guards outbox writes after close
}

func (a *agent) ID() string            { return a.id }
func (a *agent) System() System        { return a.system }
func (a *agent) Tools() Tools          { return a.tools }
func (a *agent) Hooks() Hooks          { return a.hooks }
func (a *agent) Inbox() chan<- Message { return a.inbox }
func (a *agent) Outbox() <-chan Event  { return a.outbox }
func (a *agent) Messages() []Message   { return a.snapshot() }

func (a *agent) SetMessages(msgs []Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = make([]Message, len(msgs))
	copy(a.messages, msgs)
}

// Run is the agent's main loop: wait for input → think+act → repeat.
// Outbox is closed when Run returns. Inbox is NOT closed (caller owns it).
func (a *agent) Run(ctx context.Context) error {
	a.emit(ctx, StartEvent(a.id))

	var runErr error
	defer func() {
		// StopEvent must be delivered even on context cancellation,
		// so use emitFinal which bypasses ctx.Done().
		a.emitFinal(StopEvent(a.id, runErr))
		a.closed.Store(true)

		// Drain async hooks before closing outbox to prevent
		// late writes to a closed channel.
		if a.hooks != nil {
			a.hooks.Drain()
		}
		close(a.outbox)
	}()

	for {
		if err := a.waitForInput(ctx); err != nil {
			if err == errStopped {
				return nil
			}
			runErr = err
			return err
		}

		if err := a.thinkAct(ctx); err != nil {
			if err == errStopped {
				return nil
			}
			runErr = err
			return err
		}
	}
}

var errStopped = errors.New("stopped")

// TruncatedResumePrompt is injected when generation stops at the output limit
// and the caller wants the model to continue in the next turn.
const TruncatedResumePrompt = "Your response was truncated due to output token limits. Resume directly from where you left off. Do not repeat any content."

// waitForInput blocks until at least one message arrives, then drains remaining.
func (a *agent) waitForInput(ctx context.Context) error {
	// Block until first message
	select {
	case msg, ok := <-a.inbox:
		if !ok || msg.Signal == SigStop {
			return errStopped
		}
		a.ingest(ctx, msg)
	case <-ctx.Done():
		return ctx.Err()
	}

	// Drain remaining (non-blocking)
	for {
		select {
		case msg, ok := <-a.inbox:
			if !ok || msg.Signal == SigStop {
				return errStopped
			}
			a.ingest(ctx, msg)
		default:
			return nil
		}
	}
}

// ingest notifies hooks and appends a message (with text + images) to conversation.
func (a *agent) ingest(ctx context.Context, msg Message) {
	a.emit(ctx, MessageEvent(msg))
	if msg.Signal == "" {
		a.append(msg)
	}
}

// thinkAct runs the LLM inference loop until end_turn.
func (a *agent) thinkAct(ctx context.Context) error {
	var turns, toolUses, tokensIn, tokensOut int
	var maxOutputRecoveryCount int

	for {
		if ctx.Err() != nil {
			a.emit(ctx, TurnEvent(a.id, Result{
				Messages: a.snapshot(),
				Turns: turns, ToolUses: toolUses, TokensIn: tokensIn, TokensOut: tokensOut,
				StopReason: StopCancelled,
			}))
			return ctx.Err()
		}

		// Max turns guard
		if a.maxTurns > 0 && turns >= a.maxTurns {
			a.emit(ctx, TurnEvent(a.id, Result{
				Content: "max turns reached", Messages: a.snapshot(),
				Turns: turns, ToolUses: toolUses, TokensIn: tokensIn, TokensOut: tokensOut,
				StopReason: StopMaxTurns,
			}))
			break
		}

		// Between turns: drain any new inbox messages (non-blocking)
		if turns > 0 {
			if err := a.drainInbox(ctx); err != nil {
				return err
			}
		}

		// PreInfer hook — can inject context, block, or compact (via SetMessages)
		action := a.fire(ctx, PreInferEvent(a.id))
		if action.Block {
			a.emit(ctx, TurnEvent(a.id, Result{
				Messages: a.snapshot(),
				Turns: turns, ToolUses: toolUses, TokensIn: tokensIn, TokensOut: tokensOut,
				StopReason: StopHook,
				StopDetail: action.Reason,
			}))
			break
		}
		if action.Inject != "" {
			// Fixed name so each turn's injection replaces the previous one.
			// Hook-injected context is ephemeral — it applies to the next
			// LLM call only, not accumulated across turns.
			a.system.Set(Layer{
				Name: "hook-inject", Priority: 650, Content: action.Inject, Source: Dynamic,
			})
		}

		resp, err := a.streamInfer(ctx)
		if err != nil {
			return err
		}

		turns++
		tokensIn += resp.TokensIn
		tokensOut += resp.TokensOut

		a.emit(ctx, PostInferEvent(a.id, resp))
		a.append(Message{
			Role: RoleAssistant, From: a.id,
			Content: resp.Content, Thinking: resp.Thinking,
			ThinkingSignature: resp.ThinkingSignature,
			ToolCalls:         resp.ToolCalls,
		})

		// Max tokens recovery — output truncated, ask LLM to continue
		if resp.StopReason == StopMaxTokens && len(resp.ToolCalls) == 0 {
			maxRecovery := a.maxOutputRecovery
			if maxRecovery <= 0 {
				maxRecovery = 3
			}
			if maxOutputRecoveryCount >= maxRecovery {
				a.emit(ctx, TurnEvent(a.id, Result{
					Content: resp.Content, Messages: a.snapshot(),
					Turns: turns, ToolUses: toolUses, TokensIn: tokensIn, TokensOut: tokensOut,
					StopReason: StopMaxOutputRecoveryExhausted,
				}))
				break
			}
			maxOutputRecoveryCount++
			a.append(Message{Role: RoleUser, From: "system", Content: TruncatedResumePrompt})
			continue
		}

		// No tool calls → end turn
		if len(resp.ToolCalls) == 0 {
			a.emit(ctx, TurnEvent(a.id, Result{
				Content:    resp.Content,
				Messages:   a.snapshot(),
				Turns:      turns,
				ToolUses:   toolUses,
				TokensIn:   tokensIn,
				TokensOut:  tokensOut,
				StopReason: StopEndTurn,
			}))
			break
		}

		// Execute tool calls
		toolUses += a.execTools(ctx, resp.ToolCalls)
	}

	return nil
}

// execTools runs tool calls in three phases:
//  1. Permission check (runs first) + PreTool hooks — sequential
//  2. Execute — parallel when multiple tools pass
//  3. Record results + PostTool hooks — sequential, in original call order
func (a *agent) execTools(ctx context.Context, calls []ToolCall) int {
	// Phase 1: Permission + PreTool hooks (sequential)
	type task struct {
		call ToolCall
		tool Tool
	}
	var tasks []task
	for _, tc := range calls {
		if ctx.Err() != nil {
			break
		}
		// Permission check runs before hooks to gate on the original input.
		if a.permission != nil {
			if allow, reason := a.permission(ctx, tc); !allow {
				a.appendResult(tc, "blocked: "+reason, true)
				continue
			}
		}
		action := a.fire(ctx, PreToolEvent(tc))
		if action.Block {
			a.appendResult(tc, "blocked: "+action.Reason, true)
			continue
		}
		if action.Modify != nil {
			if modifiedJSON, err := json.Marshal(action.Modify); err == nil {
				tc.Input = string(modifiedJSON)
			}
		}
		t := a.tools.Get(tc.Name)
		if t == nil {
			a.appendResult(tc, fmt.Sprintf("unknown tool: %s", tc.Name), true)
			continue
		}
		tasks = append(tasks, task{tc, t})
	}
	if len(tasks) == 0 {
		return 0
	}

	// Phase 2: Execute (parallel when multiple, direct when single)
	type output struct {
		content string
		err     error
	}
	results := make([]output, len(tasks))
	if len(tasks) == 1 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("core/agent: tool %s panicked: %v\n%s", tasks[0].call.Name, r, debug.Stack())
					results[0] = output{"", fmt.Errorf("tool %s panicked: %v", tasks[0].call.Name, r)}
				}
			}()
			params, _ := ParseToolInput(tasks[0].call.Input)
			execCtx := WithToolCallID(ctx, tasks[0].call.ID)
			content, err := tasks[0].tool.Execute(execCtx, params)
			results[0] = output{content, err}
		}()
	} else {
		var wg sync.WaitGroup
		for i, t := range tasks {
			wg.Add(1)
			go func(i int, t task) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("core/agent: tool %s panicked: %v\n%s", t.call.Name, r, debug.Stack())
						results[i] = output{"", fmt.Errorf("tool %s panicked: %v", t.call.Name, r)}
					}
				}()
				params, _ := ParseToolInput(t.call.Input)
				execCtx := WithToolCallID(ctx, t.call.ID)
				content, err := t.tool.Execute(execCtx, params)
				results[i] = output{content, err}
			}(i, t)
		}
		wg.Wait()
	}

	// Phase 3: Record results in order + PostTool hooks
	var toolUses int
	for i, t := range tasks {
		r := results[i]
		if r.err != nil {
			a.appendResult(t.call, r.err.Error(), true)
			a.emit(ctx, PostToolEvent(ToolResult{
				ToolCallID: t.call.ID, ToolName: t.call.Name, Content: r.err.Error(), IsError: true,
			}))
			continue
		}
		toolUses++
		a.appendResult(t.call, r.content, false)
		a.emit(ctx, PostToolEvent(ToolResult{
			ToolCallID: t.call.ID, ToolName: t.call.Name, Content: r.content,
		}))
	}
	return toolUses
}

// --- context keys ---

type contextKey string

const toolCallIDKey contextKey = "tool_call_id"

// WithToolCallID returns a context carrying the given tool call ID.
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey, id)
}

// ToolCallIDFromContext extracts the tool call ID from the context.
func ToolCallIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(toolCallIDKey).(string); ok {
		return id
	}
	return ""
}

// --- internals ---

// streamInfer calls the LLM, streams chunks to outbox, returns the final response.
func (a *agent) streamInfer(ctx context.Context) (*InferResponse, error) {
	chunks, err := a.llm.Infer(ctx, InferRequest{
		System:   a.system.Prompt(),
		Messages: a.snapshot(),
		Tools:    a.tools.Schemas(),
	})
	if err != nil {
		return nil, fmt.Errorf("infer: %w", err)
	}

	var resp *InferResponse
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				if resp == nil {
					return nil, fmt.Errorf("infer: stream closed without response")
				}
				return resp, nil
			}
			if chunk.Err != nil {
				return nil, fmt.Errorf("infer: %w", chunk.Err)
			}
			if chunk.Text != "" || chunk.Thinking != "" {
				a.emit(ctx, ChunkEvent(a.id, chunk))
			}
			if chunk.Done {
				resp = chunk.Response
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// emit sends an event to the outbox for external observation.
// Blocks if outbox is full (backpressure). Skips if outbox is closed or ctx is cancelled.
func (a *agent) emit(ctx context.Context, event Event) {
	if a.closed.Load() {
		return
	}
	select {
	case a.outbox <- event:
	case <-ctx.Done():
	}
}

// emitFinal sends a critical event that must be delivered even on ctx cancellation.
// Used for StopEvent — consumers rely on it for cleanup/session saving.
// Blocks up to 5 seconds; logs a warning if delivery fails.
func (a *agent) emitFinal(event Event) {
	if a.closed.Load() {
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case a.outbox <- event:
	case <-timer.C:
		log.Printf("core/agent: failed to deliver %s event (outbox full for 5s)", event.Type)
	}
}

// fire dispatches an event to hooks and returns the merged Action.
// Also emits the event to outbox for observation.
// Hook errors are treated as Block (fail-closed for safety).
func (a *agent) fire(ctx context.Context, event Event) Action {
	a.emit(ctx, event)
	if a.hooks == nil {
		return Action{}
	}
	action, err := a.hooks.Fire(ctx, event)
	if err != nil {
		log.Printf("core/agent: hook error on %s: %v (treating as block)", event.Type, err)
		action.Block = true
		if action.Reason == "" {
			action.Reason = err.Error()
		}
	}
	return action
}

// drainInbox non-blocking reads all pending inbox messages.
// Returns errStopped if SigStop is received.
func (a *agent) drainInbox(ctx context.Context) error {
	for {
		select {
		case msg, ok := <-a.inbox:
			if !ok || msg.Signal == SigStop {
				return errStopped
			}
			a.ingest(ctx, msg)
		default:
			return nil
		}
	}
}

func (a *agent) append(msg Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = append(a.messages, msg)
}

func (a *agent) snapshot() []Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cp := make([]Message, len(a.messages))
	copy(cp, a.messages)
	return cp
}

func (a *agent) appendResult(tc ToolCall, content string, isError bool) {
	a.append(Message{
		Role: RoleTool, From: tc.Name, Content: content,
		ToolResult: &ToolResult{ToolCallID: tc.ID, ToolName: tc.Name, Content: content, IsError: isError},
	})
}

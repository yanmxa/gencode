// Package runtime provides a reusable agent loop that manages conversation state
// and orchestrates LLM interactions. It serves as the runtime for all agent types:
// subagents, the TUI, and custom agents.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

const (
	defaultMaxTurns          = 100
	minMessagesForCompaction = 3

	// DefaultMaxOutputRecovery is the default number of retries when LLM output
	// is truncated due to max_tokens. Exported so the TUI layer can reuse it.
	DefaultMaxOutputRecovery = 3
)

// MaxOutputRecoveryPrompt is the message injected when the LLM output is truncated.
const MaxOutputRecoveryPrompt = "Your response was truncated due to output token limits. Resume directly from where you left off. Do not repeat any content."

// AutoCompactResumePrompt is the user message injected after an auto-compaction
// when the caller should continue the task immediately.
const AutoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

// Stop reason constants returned in Result.StopReason.
const (
	StopEndTurn                    = "end_turn"
	StopMaxTurns                   = "max_turns"
	StopCancelled                  = "cancelled"
	StopHook                       = "stop_hook"
	StopMaxOutputRecoveryExhausted = "max_output_recovery_exhausted"
)

// TransitionReason describes why the loop moved to its current state.
type TransitionReason string

const (
	TransitionNextTurn          TransitionReason = "next_turn"
	TransitionMaxOutputRecovery TransitionReason = "max_output_tokens_recovery"
	TransitionPromptTooLong     TransitionReason = "prompt_too_long_compact"
	TransitionStopHookBlocking  TransitionReason = "stop_hook_blocking"
)

// loopState holds the explicit state of the agent loop per iteration.
type loopState struct {
	turnCount                    int
	maxOutputTokensRecoveryCount int
	transition                   TransitionReason
}

// CompletionAction describes what the runtime should do after receiving a completed response.
type CompletionAction int

const (
	CompletionEndTurn CompletionAction = iota
	CompletionRunTools
	CompletionRecoverMaxTokens
	CompletionStopMaxOutputRecovery
)

// CompletionDecision is the shared decision result used by both the synchronous
// core loop and the TUI-driven incremental loop.
type CompletionDecision struct {
	Action    CompletionAction
	ToolCalls []message.ToolCall
}

// RunOptions controls the synchronous Run() loop.
type RunOptions struct {
	MaxTurns            int
	MaxOutputRecovery   int // max retries on truncated output (default: 3)
	OnResponse          func(resp *message.CompletionResponse)
	OnToolStart         func(tc message.ToolCall) bool
	OnToolDone          func(tc message.ToolCall, result message.ToolResult)
	DrainInjectedInputs func() []string

	// Context compaction support (opt-in)
	SessionMemory string // previous compaction summary
	CompactFocus  string // optional focus for compaction
	InputLimit    int    // context window input limit (enables pre-stream check & reactive compact)
}

// Result is returned by Loop.Run() upon completion.
type Result struct {
	Content     string
	Messages    []message.Message
	Turns       int
	ToolUses    int
	Tokens      client.TokenUsage
	StopReason  string
	Transitions []TransitionReason
	StopDetail  string
}

// --- Loop ---

// MCPCaller can execute MCP tool calls. This interface decouples runtime.Loop
// from the mcp package to avoid import cycles.
type MCPCaller interface {
	// CallTool calls an MCP tool by its full name (mcp__server__tool).
	CallTool(ctx context.Context, fullName string, arguments map[string]any) (content string, isError bool, err error)
	// IsMCPTool returns true if the name is an MCP tool (mcp__*__*).
	IsMCPTool(name string) bool
}

// Loop is a reusable agent runtime that manages conversation state
// and orchestrates LLM interactions. It supports two execution models:
//
//	Synchronous: loop.Run(ctx, opts) — drives the full turn loop
//	Incremental: loop.Stream()/Collect()/AddResponse()/FilterToolCalls()/ExecTool() — for event-driven callers
type Loop struct {
	System          *system.System
	Client          *client.Client
	Tool            *tool.Set
	Permission      permission.Checker
	Hooks           *hooks.Engine
	MCP             MCPCaller // optional: routes mcp__*__* tool calls
	QuestionHandler tool.AskQuestionFunc

	// Agent context: when set, tool hook events include agent_id/agent_type (subagent mode)
	AgentID   string
	AgentType string

	// State (managed by the loop)
	messages []message.Message
}

// --- High-level: synchronous agent loop ---

// Run drives the full conversation loop using an explicit state machine:
// while-true { pre-check → stream → error recovery → tools → next state }.
// Stops on end_turn, max turns, stop hook, recovery exhaustion, or context cancellation.
func (l *Loop) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}
	maxRecovery := opts.MaxOutputRecovery
	if maxRecovery <= 0 {
		maxRecovery = DefaultMaxOutputRecovery
	}

	state := loopState{
		turnCount:  1,
		transition: TransitionNextTurn,
	}
	var toolUses int
	var transitions []TransitionReason

	terminate := func(reason string) *Result {
		return l.buildResult(reason, state.turnCount, toolUses, transitions)
	}

	for {
		// --- 0. Context cancellation check ---
		select {
		case <-ctx.Done():
			return terminate(StopCancelled), ctx.Err()
		default:
		}

		transitions = append(transitions, state.transition)

		if opts.DrainInjectedInputs != nil {
			for _, injected := range opts.DrainInjectedInputs() {
				if injected == "" {
					continue
				}
				l.AddUser(injected, nil)
			}
		}

		// --- 1. Pre-stream: reactive compaction (prevent prompt-too-long) ---
		if opts.InputLimit > 0 && l.Client != nil {
			tokens := l.Client.Tokens()
			if message.NeedsCompaction(tokens.InputTokens, opts.InputLimit) && CanCompactMessages(len(l.messages)) {
				if l.compactAndReplace(ctx, opts) {
					state.transition = TransitionPromptTooLong
					continue
				}
			}
		}

		// --- 2. Stream + collect response ---
		resp, err := Collect(ctx, l.Stream(ctx))
		if err != nil {
			if ShouldCompactPromptTooLong(err, len(l.messages)) {
				if l.compactAndReplace(ctx, opts) {
					state.transition = TransitionPromptTooLong
					continue
				}
			}
			return nil, err
		}

		// --- 3. Process response ---
		calls := l.AddResponse(resp)
		if opts.OnResponse != nil {
			opts.OnResponse(resp)
		}

		decision := DecideCompletion(resp.StopReason, calls, state.maxOutputTokensRecoveryCount, maxRecovery)

		// --- 3a. Max-output-tokens recovery ---
		if decision.Action == CompletionRecoverMaxTokens {
			if state.maxOutputTokensRecoveryCount < maxRecovery {
				l.AddUser(MaxOutputRecoveryPrompt, nil)
				state.maxOutputTokensRecoveryCount++
				state.transition = TransitionMaxOutputRecovery
				continue
			}
		}
		if decision.Action == CompletionStopMaxOutputRecovery {
			return terminate(StopMaxOutputRecoveryExhausted), nil
		}

		// --- 4. No tool calls → fire stop hooks, then end ---
		if decision.Action == CompletionEndTurn {
			if l.Hooks != nil && l.Hooks.HasHooks(hooks.Stop) {
				stopInput := hooks.HookInput{
					LastAssistantMessage: l.lastAssistantContent(),
					StopHookActive:       l.Hooks.StopHookActive(),
				}
				outcome := l.Hooks.Execute(ctx, hooks.Stop, stopInput)
				if outcome.ShouldBlock {
					r := terminate(StopHook)
					r.StopDetail = "Stop hook blocked: " + outcome.BlockReason
					r.Transitions = append(r.Transitions, TransitionStopHookBlocking)
					return r, nil
				}
			}
			return terminate(StopEndTurn), nil
		}

		// --- 5. Filter through hooks ---
		allowed, blocked, _, hookContext := l.FilterToolCalls(ctx, decision.ToolCalls)
		_ = hookContext // context injection handled by incremental callers
		for _, br := range blocked {
			l.AddToolResult(br)
		}

		// --- 6. Execute tools ---
		for _, tc := range allowed {
			select {
			case <-ctx.Done():
				return terminate(StopCancelled), ctx.Err()
			default:
			}

			if opts.OnToolStart != nil && !opts.OnToolStart(tc) {
				continue
			}

			result := l.ExecTool(ctx, tc)
			l.AddToolResult(*result)
			toolUses++

			l.firePostToolHook(ctx, tc, result)

			if opts.OnToolDone != nil {
				opts.OnToolDone(tc, *result)
			}
		}

		// --- 7. Turn limit check ---
		if state.turnCount >= maxTurns {
			return terminate(StopMaxTurns), nil
		}

		// --- 8. Build next state ---
		state = loopState{
			turnCount:                    state.turnCount + 1,
			maxOutputTokensRecoveryCount: state.maxOutputTokensRecoveryCount,
			transition:                   TransitionNextTurn,
		}
	}
}

func (l *Loop) buildResult(reason string, turns, toolUses int, transitions []TransitionReason) *Result {
	return &Result{
		Content:     l.lastAssistantContent(),
		Messages:    l.messages,
		Turns:       turns,
		ToolUses:    toolUses,
		Tokens:      l.Client.Tokens(),
		StopReason:  reason,
		Transitions: transitions,
	}
}

// --- Low-level: incremental control (for TUI / event-driven callers) ---

// Stream starts an LLM stream and returns the chunk channel.
// It builds the system prompt and tool set from the loop's fields.
func (l *Loop) Stream(ctx context.Context) <-chan message.StreamChunk {
	sysPrompt := l.System.Prompt()
	tools := l.Tool.Tools()
	return l.Client.Stream(ctx, l.messages, tools, sysPrompt)
}

// --- Message management ---

// Messages returns the current conversation history (read-only; do not mutate).
func (l *Loop) Messages() []message.Message { return l.messages }

// SetMessages replaces the conversation history. Used for session restore and forking.
func (l *Loop) SetMessages(msgs []message.Message) { l.messages = msgs }

// Tokens returns the cumulative token usage tracked by the underlying client.
// Returns a zero value if no client is attached.
func (l *Loop) Tokens() client.TokenUsage {
	if l.Client == nil {
		return client.TokenUsage{}
	}
	return l.Client.Tokens()
}

// AddUser appends a user message (text + optional images) to the conversation.
func (l *Loop) AddUser(content string, images []message.ImageData) {
	l.messages = append(l.messages, message.UserMessage(content, images))
}

// AddResponse appends the assistant message, updates token counters, and returns tool calls.
func (l *Loop) AddResponse(resp *message.CompletionResponse) []message.ToolCall {
	if l.Client != nil {
		l.Client.AddUsage(resp.Usage)
	}

	l.messages = append(l.messages, message.AssistantMessage(resp.Content, resp.Thinking, resp.ToolCalls))

	return resp.ToolCalls
}

// AddToolResult appends a tool result message to the conversation.
func (l *Loop) AddToolResult(r message.ToolResult) {
	l.messages = append(l.messages, message.ToolResultMessage(r))
}

// --- Tool dispatch ---

// FilterToolCallsResult holds the results from PreToolUse hook filtering.
type FilterToolCallsResult struct {
	Allowed           []message.ToolCall
	Blocked           []message.ToolResult
	HookAllowed       map[string]bool // tool call IDs pre-approved by hooks
	HookForceAsk      map[string]bool // tool call IDs forced to prompt by hooks ("ask")
	AdditionalContext string
}

// FilterToolCalls runs PreToolUse hooks. Convenience wrapper for backward compat.
func (l *Loop) FilterToolCalls(ctx context.Context, calls []message.ToolCall) (
	allowed []message.ToolCall, blocked []message.ToolResult, hookAllowed map[string]bool, additionalContext string,
) {
	r := l.FilterToolCallsEx(ctx, calls)
	return r.Allowed, r.Blocked, r.HookAllowed, r.AdditionalContext
}

// FilterToolCallsEx runs PreToolUse hooks, returning full results including ForceAsk.
//
// PreToolUse hooks can grant/deny/force-ask permissions, but cannot inject
// updatedPermissions (setMode, addRules, etc.) — that's PermissionRequest-only (matches CC).
func (l *Loop) FilterToolCallsEx(ctx context.Context, calls []message.ToolCall) FilterToolCallsResult {
	r := FilterToolCallsResult{
		HookAllowed:  make(map[string]bool),
		HookForceAsk: make(map[string]bool),
	}
	if l.Hooks == nil {
		r.Allowed = calls
		return r
	}

	for _, tc := range calls {
		params, _ := message.ParseToolInput(tc.Input)
		hookInput := hooks.HookInput{
			ToolName:  tc.Name,
			ToolInput: params,
			ToolUseID: tc.ID,
		}
		if l.AgentID != "" {
			hookInput.AgentID = l.AgentID
			hookInput.AgentType = l.AgentType
		}
		outcome := l.Hooks.Execute(ctx, hooks.PreToolUse, hookInput)

		if outcome.ShouldBlock {
			r.Blocked = append(r.Blocked, *message.ErrorResult(tc, "Blocked by hook: "+outcome.BlockReason))
			continue
		}

		if outcome.UpdatedInput != nil {
			if updated, err := json.Marshal(outcome.UpdatedInput); err == nil {
				tc.Input = string(updated)
			}
		}

		if outcome.AdditionalContext != "" {
			if r.AdditionalContext == "" {
				r.AdditionalContext = outcome.AdditionalContext
			} else {
				r.AdditionalContext += "\n" + outcome.AdditionalContext
			}
		}

		if outcome.PermissionAllow {
			r.HookAllowed[tc.ID] = true
		}
		if outcome.ForceAsk {
			r.HookForceAsk[tc.ID] = true
		}

		r.Allowed = append(r.Allowed, tc)
	}
	return r
}

// firePostToolHook fires PostToolUse or PostToolUseFailure hooks after tool execution.
func (l *Loop) firePostToolHook(ctx context.Context, tc message.ToolCall, result *message.ToolResult) {
	if l.Hooks == nil {
		return
	}

	params, _ := message.ParseToolInput(tc.Input)
	event := hooks.PostToolUse
	if result.IsError {
		event = hooks.PostToolUseFailure
	}

	toolResponse := any(result.Content)
	if result.HookResponse != nil {
		toolResponse = result.HookResponse
	}
	input := hooks.HookInput{
		ToolName:     tc.Name,
		ToolInput:    params,
		ToolUseID:    tc.ID,
		ToolResponse: toolResponse,
	}
	if l.AgentID != "" {
		input.AgentID = l.AgentID
		input.AgentType = l.AgentType
	}
	if result.IsError {
		input.Error = result.Content
	}

	l.Hooks.ExecuteAsync(event, input)
}

// ExecTool executes a single tool call, consulting the Permission checker.
// Rejected tools return an error result; Prompt decisions are auto-approved.
func (l *Loop) ExecTool(ctx context.Context, tc message.ToolCall) *message.ToolResult {
	prepared, err := tool.PrepareToolCall(tc, mcpAdapter{caller: l.MCP})
	if err != nil {
		return message.ErrorResult(tc, fmt.Sprintf("Error parsing tool input: %v", err))
	}

	// Inject parent messages getter for fork support in Agent tools
	if tool.IsAgentToolName(prepared.Call.Name) {
		prepared.Params["_messagesGetter"] = tool.MessagesGetter(func() []message.Message {
			// Return a copy to avoid concurrent mutation
			msgs := make([]message.Message, len(l.messages))
			copy(msgs, l.messages)
			return msgs
		})
	}

	decision := permission.Permit
	if l.Permission != nil {
		decision = l.Permission.Check(prepared.Call.Name, prepared.Params)
	}

	if decision == permission.Reject {
		return message.ErrorResult(tc, fmt.Sprintf("Tool %s is not permitted in this mode", tc.Name))
	}

	// Permit and Prompt both execute the tool (non-interactive callers auto-approve)
	return l.runTool(ctx, prepared)
}

// runTool runs the actual tool execution.
func (l *Loop) runTool(ctx context.Context, prepared *tool.PreparedToolCall) *message.ToolResult {
	cwd := ""
	if l.System != nil {
		cwd = l.System.Cwd
	}

	if it, ok := prepared.Tool.(tool.InteractiveTool); ok && it.RequiresInteraction() {
		req, err := it.PrepareInteraction(ctx, prepared.Params, cwd)
		if err != nil {
			return message.ErrorResult(prepared.Call, fmt.Sprintf("Error preparing interactive tool: %v", err))
		}

		questionReq, ok := req.(*tool.QuestionRequest)
		if !ok {
			return message.ErrorResult(prepared.Call, fmt.Sprintf("interactive tool %s is not supported in this runtime", prepared.Call.Name))
		}
		if l.QuestionHandler == nil {
			return message.ErrorResult(prepared.Call, fmt.Sprintf("interactive tool %s requires a question handler in this runtime", prepared.Call.Name))
		}

		resp, err := l.QuestionHandler(ctx, questionReq)
		if err != nil {
			return message.ErrorResult(prepared.Call, fmt.Sprintf("Question prompt failed: %v", err))
		}
		if resp == nil {
			return message.ErrorResult(prepared.Call, "Question prompt failed: no response received")
		}

		toolResult := it.ExecuteWithResponse(ctx, prepared.Params, resp, cwd)
		return &message.ToolResult{
			ToolCallID:   prepared.Call.ID,
			ToolName:     prepared.Call.Name,
			Content:      toolResult.FormatForLLM(),
			IsError:      !toolResult.Success,
			HookResponse: toolResult.HookResponse,
		}
	}

	toolResult, err := prepared.Execute(ctx, cwd, true, mcpAdapter{caller: l.MCP})
	if err != nil {
		if prepared.IsMCP {
			return message.ErrorResult(prepared.Call, fmt.Sprintf("MCP tool error: %v", err))
		}
		return message.ErrorResult(prepared.Call, fmt.Sprintf("Unknown tool: %s", prepared.Call.Name))
	}

	log.Logger().Debug("Tool executed",
		zap.String("tool", prepared.Call.Name),
		zap.Bool("success", toolResult.Success),
	)

	return &message.ToolResult{
		ToolCallID:   prepared.Call.ID,
		ToolName:     prepared.Call.Name,
		Content:      toolResult.FormatForLLM(),
		IsError:      !toolResult.Success,
		HookResponse: toolResult.HookResponse,
	}
}

type mcpAdapter struct {
	caller MCPCaller
}

func (a mcpAdapter) IsMCPTool(name string) bool {
	return a.caller != nil && a.caller.IsMCPTool(name)
}

func (a mcpAdapter) ExecuteMCP(ctx context.Context, name string, params map[string]any) (toolresult.ToolResult, error) {
	if a.caller == nil {
		return toolresult.ToolResult{}, fmt.Errorf("MCP caller not configured")
	}

	content, isError, err := a.caller.CallTool(ctx, name, params)
	if err != nil {
		return toolresult.ToolResult{}, err
	}

	return toolresult.ToolResult{
		Success: !isError,
		Output:  content,
		Metadata: toolresult.ResultMetadata{
			Title: name,
		},
	}, nil
}

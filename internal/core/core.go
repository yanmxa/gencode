// Package core provides a reusable agent loop that manages conversation state
// and orchestrates LLM interactions. It serves as the runtime for all agent types:
// subagents, the TUI, and custom agents.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	defaultMaxTurns = 100

	// DefaultMaxOutputRecovery is the default number of retries when LLM output
	// is truncated due to max_tokens. Exported so the TUI layer can reuse it.
	DefaultMaxOutputRecovery = 3
)

// MaxOutputRecoveryPrompt is the message injected when the LLM output is truncated.
const MaxOutputRecoveryPrompt = "Your response was truncated due to output token limits. Resume directly from where you left off. Do not repeat any content."

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
	MaxTurns          int
	MaxOutputRecovery int // max retries on truncated output (default: 3)
	OnResponse        func(resp *message.CompletionResponse)
	OnToolStart       func(tc message.ToolCall) bool
	OnToolDone        func(tc message.ToolCall, result message.ToolResult)

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

// MCPCaller can execute MCP tool calls. This interface decouples core.Loop
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
	System     *system.System
	Client     *client.Client
	Tool       *tool.Set
	Permission permission.Checker
	Hooks      *hooks.Engine
	MCP        MCPCaller // optional: routes mcp__*__* tool calls

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

		// --- 1. Pre-stream: reactive compaction (prevent prompt-too-long) ---
		if opts.InputLimit > 0 && l.Client != nil {
			tokens := l.Client.Tokens()
			if message.NeedsCompaction(tokens.InputTokens, opts.InputLimit) && len(l.messages) >= 3 {
				if l.compactAndReplace(ctx, opts) {
					state.transition = TransitionPromptTooLong
					continue
				}
			}
		}

		// --- 2. Stream + collect response ---
		resp, err := Collect(ctx, l.Stream(ctx))
		if err != nil {
			if IsPromptTooLong(err) && len(l.messages) >= 3 {
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
					StopHookActive:       true,
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

// DecideCompletion determines the next action after a completed assistant response.
func DecideCompletion(stopReason string, calls []message.ToolCall, recoveryCount, maxRecovery int) CompletionDecision {
	if stopReason == "max_tokens" && len(calls) == 0 {
		if recoveryCount < maxRecovery {
			return CompletionDecision{Action: CompletionRecoverMaxTokens}
		}
		return CompletionDecision{Action: CompletionStopMaxOutputRecovery}
	}

	if len(calls) > 0 {
		return CompletionDecision{
			Action:    CompletionRunTools,
			ToolCalls: calls,
		}
	}

	return CompletionDecision{Action: CompletionEndTurn}
}

func (l *Loop) lastAssistantContent() string {
	for i := len(l.messages) - 1; i >= 0; i-- {
		msg := l.messages[i]
		if msg.Role == message.RoleAssistant && msg.Content != "" {
			return msg.Content
		}
	}
	return ""
}

// --- Low-level: incremental control (for TUI / event-driven callers) ---

// Stream starts an LLM stream and returns the chunk channel.
// It builds the system prompt and tool set from the loop's fields.
func (l *Loop) Stream(ctx context.Context) <-chan message.StreamChunk {
	sysPrompt := l.System.Prompt()
	tools := l.Tool.Tools()
	return l.Client.Stream(ctx, l.messages, tools, sysPrompt)
}

// Collect synchronously drains a stream into a CompletionResponse.
func Collect(ctx context.Context, ch <-chan message.StreamChunk) (*message.CompletionResponse, error) {
	var response message.CompletionResponse

	for chunk := range ch {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch chunk.Type {
		case message.ChunkTypeText:
			response.Content += chunk.Text
		case message.ChunkTypeThinking:
			response.Thinking += chunk.Text
		case message.ChunkTypeToolStart:
			response.ToolCalls = append(response.ToolCalls, message.ToolCall{
				ID:   chunk.ToolID,
				Name: chunk.ToolName,
			})
		case message.ChunkTypeToolInput:
			if len(response.ToolCalls) > 0 {
				idx := len(response.ToolCalls) - 1
				response.ToolCalls[idx].Input += chunk.Text
			}
		case message.ChunkTypeDone:
			if chunk.Response != nil {
				return chunk.Response, nil
			}
			return &response, nil
		case message.ChunkTypeError:
			return nil, chunk.Error
		}
	}

	return &response, nil
}

// --- Message management ---

func (l *Loop) Messages() []message.Message        { return l.messages }
func (l *Loop) SetMessages(msgs []message.Message) { l.messages = msgs }

func (l *Loop) Tokens() client.TokenUsage {
	if l.Client == nil {
		return client.TokenUsage{}
	}
	return l.Client.Tokens()
}

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

func (l *Loop) AddToolResult(r message.ToolResult) {
	l.messages = append(l.messages, message.ToolResultMessage(r))
}

// --- Tool dispatch ---

// FilterToolCalls runs PreToolUse hooks, returning allowed tool calls, blocked results,
// a set of tool call IDs that hooks explicitly allowed (can skip permission prompts),
// and any additional context injected by hooks.
func (l *Loop) FilterToolCalls(ctx context.Context, calls []message.ToolCall) (
	allowed []message.ToolCall, blocked []message.ToolResult, hookAllowed map[string]bool, additionalContext string,
) {
	if l.Hooks == nil {
		return calls, nil, nil, ""
	}

	hookAllowed = make(map[string]bool)

	for _, tc := range calls {
		params, _ := message.ParseToolInput(tc.Input)
		outcome := l.Hooks.Execute(ctx, hooks.PreToolUse, hooks.HookInput{
			ToolName:  tc.Name,
			ToolInput: params,
			ToolUseID: tc.ID,
		})

		if outcome.ShouldBlock {
			blocked = append(blocked, *message.ErrorResult(tc, "Blocked by hook: "+outcome.BlockReason))
			continue
		}

		if outcome.UpdatedInput != nil {
			if updated, err := json.Marshal(outcome.UpdatedInput); err == nil {
				tc.Input = string(updated)
			}
		}

		if outcome.AdditionalContext != "" {
			if additionalContext == "" {
				additionalContext = outcome.AdditionalContext
			} else {
				additionalContext += "\n" + outcome.AdditionalContext
			}
		}

		if outcome.PermissionAllow {
			hookAllowed[tc.ID] = true
		}

		allowed = append(allowed, tc)
	}
	return allowed, blocked, hookAllowed, additionalContext
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
	if result.IsError {
		input.Error = result.Content
	}

	l.Hooks.ExecuteAsync(event, input)
}

// ExecTool executes a single tool call, consulting the Permission checker.
// Rejected tools return an error result; Prompt decisions are auto-approved.
func (l *Loop) ExecTool(ctx context.Context, tc message.ToolCall) *message.ToolResult {
	params, err := message.ParseToolInput(tc.Input)
	if err != nil {
		return message.ErrorResult(tc, fmt.Sprintf("Error parsing tool input: %v", err))
	}

	decision := permission.Permit
	if l.Permission != nil {
		decision = l.Permission.Check(tc.Name, params)
	}

	if decision == permission.Reject {
		return message.ErrorResult(tc, fmt.Sprintf("Tool %s is not permitted in this mode", tc.Name))
	}

	// Permit and Prompt both execute the tool (non-interactive callers auto-approve)
	return l.runTool(ctx, tc, params)
}

// runTool runs the actual tool execution.
func (l *Loop) runTool(ctx context.Context, tc message.ToolCall, params map[string]any) *message.ToolResult {
	cwd := ""
	if l.System != nil {
		cwd = l.System.Cwd
	}

	if _, ok := tool.Get(tc.Name); !ok && (l.MCP == nil || !l.MCP.IsMCPTool(tc.Name)) {
		return message.ErrorResult(tc, fmt.Sprintf("Unknown tool: %s", tc.Name))
	}

	toolResult, err := tool.ExecutePreparedTool(ctx, tc, params, cwd, true, mcpAdapter{caller: l.MCP})
	if err != nil {
		if l.MCP != nil && l.MCP.IsMCPTool(tc.Name) {
			return message.ErrorResult(tc, fmt.Sprintf("MCP tool error: %v", err))
		}
		return message.ErrorResult(tc, fmt.Sprintf("Unknown tool: %s", tc.Name))
	}

	log.Logger().Debug("Tool executed",
		zap.String("tool", tc.Name),
		zap.Bool("success", toolResult.Success),
	)

	return &message.ToolResult{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    toolResult.FormatForLLM(),
		IsError:    !toolResult.Success,
	}
}

type mcpAdapter struct {
	caller MCPCaller
}

func (a mcpAdapter) IsMCPTool(name string) bool {
	return a.caller != nil && a.caller.IsMCPTool(name)
}

func (a mcpAdapter) ExecuteMCP(ctx context.Context, name string, params map[string]any) (ui.ToolResult, error) {
	if a.caller == nil {
		return ui.ToolResult{}, fmt.Errorf("MCP caller not configured")
	}

	content, isError, err := a.caller.CallTool(ctx, name, params)
	if err != nil {
		return ui.ToolResult{}, err
	}

	return ui.ToolResult{
		Success: !isError,
		Output:  content,
		Metadata: ui.ResultMetadata{
			Title: name,
		},
	}, nil
}

// --- Helpers ---

// IsPromptTooLong checks if an API error indicates the prompt exceeded the context window.
func IsPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "prompt_too_long")
}

// compactAndReplace summarizes the conversation and replaces messages with the summary.
// Returns true if compaction succeeded.
func (l *Loop) compactAndReplace(ctx context.Context, opts RunOptions) bool {
	summary, _, err := Compact(ctx, l.Client, l.messages, opts.SessionMemory, opts.CompactFocus)
	if err != nil {
		return false
	}
	l.messages = []message.Message{message.UserMessage(
		"Previous context summary:\n"+summary+"\n\nContinue with the task.", nil,
	)}
	return true
}

// --- Compaction ---

// Compact summarizes a conversation to reduce context window usage.
// It sends the conversation to the LLM with a compact prompt and returns
// the summary text, the original message count, and any error.
// sessionMemory is the previous compaction summary; if non-empty it is
// prepended so the new summary incorporates prior context.
func Compact(ctx context.Context, c *client.Client,
	msgs []message.Message, sessionMemory, focus string,
) (summary string, count int, err error) {
	count = len(msgs)

	conversationText := message.BuildConversationText(msgs)

	if sessionMemory != "" {
		conversationText = fmt.Sprintf("Previous session context:\n\n%s\n\n---\n\nRecent conversation:\n\n%s", sessionMemory, conversationText)
	}

	if focus != "" {
		conversationText += fmt.Sprintf("\n\n**Important**: Focus the summary on: %s", focus)
	}

	response, err := c.Complete(ctx,
		system.CompactPrompt(),
		[]message.Message{message.UserMessage(conversationText, nil)},
		2048,
	)
	if err != nil {
		return "", count, fmt.Errorf("failed to generate summary: %w", err)
	}

	return strings.TrimSpace(response.Content), count, nil
}

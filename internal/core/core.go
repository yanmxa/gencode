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

const defaultMaxTurns = 50

// RunOptions controls the synchronous Run() loop.
type RunOptions struct {
	MaxTurns    int
	OnResponse  func(resp *message.CompletionResponse)
	OnToolStart func(tc message.ToolCall) bool
	OnToolDone  func(tc message.ToolCall, result message.ToolResult)
}

// Result is returned by Loop.Run() upon completion.
type Result struct {
	Content    string
	Messages   []message.Message
	Turns      int
	Tokens     client.TokenUsage
	StopReason string // "end_turn", "max_turns", "cancelled"
}

// --- Loop ---

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

	// State (managed by the loop)
	messages []message.Message
}

// --- High-level: synchronous agent loop ---

// Run drives the full conversation loop: stream -> response -> tools -> repeat.
// Stops on end_turn, max turns, or context cancellation.
func (l *Loop) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}

	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-ctx.Done():
			return l.buildResult("cancelled", turn), ctx.Err()
		default:
		}

		// 1. Stream + collect response
		resp, err := Collect(ctx, l.Stream(ctx))
		if err != nil {
			return nil, err
		}

		// 2. Process response
		calls := l.AddResponse(resp)
		if opts.OnResponse != nil {
			opts.OnResponse(resp)
		}

		// 3. No tool calls -> done
		if len(calls) == 0 {
			r := l.buildResult("end_turn", turn+1)
			r.Content = resp.Content
			return r, nil
		}

		// 4. Filter through hooks
		allowed, blocked := l.FilterToolCalls(ctx, calls)
		for _, br := range blocked {
			l.AddToolResult(br)
		}

		// 5. Execute tools
		for _, tc := range allowed {
			select {
			case <-ctx.Done():
				return l.buildResult("cancelled", turn+1), ctx.Err()
			default:
			}

			if opts.OnToolStart != nil && !opts.OnToolStart(tc) {
				continue
			}

			result := l.ExecTool(ctx, tc)
			l.AddToolResult(*result)
			if opts.OnToolDone != nil {
				opts.OnToolDone(tc, *result)
			}
		}
	}

	return l.buildResult("max_turns", maxTurns), nil
}

func (l *Loop) buildResult(reason string, turns int) *Result {
	return &Result{
		Content:    l.lastAssistantContent(),
		Messages:   l.messages,
		Turns:      turns,
		Tokens:     l.Client.Tokens(),
		StopReason: reason,
	}
}

// lastAssistantContent returns the content of the most recent assistant message.
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

// Messages returns the current conversation messages.
func (l *Loop) Messages() []message.Message {
	return l.messages
}

// SetMessages replaces the conversation messages.
func (l *Loop) SetMessages(msgs []message.Message) {
	l.messages = msgs
}

// Tokens returns the accumulated token usage from the client.
func (l *Loop) Tokens() client.TokenUsage {
	if l.Client == nil {
		return client.TokenUsage{}
	}
	return l.Client.Tokens()
}

// AddUser appends a user message to the conversation.
func (l *Loop) AddUser(content string, images []message.ImageData) {
	l.messages = append(l.messages, message.UserMessage(content, images))
}

// AddResponse processes a CompletionResponse: appends the assistant message
// to the conversation, updates token counters, and returns the tool calls.
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

// FilterToolCalls runs PreToolUse hooks, returning allowed tool calls and blocked results.
func (l *Loop) FilterToolCalls(ctx context.Context, calls []message.ToolCall) (
	allowed []message.ToolCall, blocked []message.ToolResult,
) {
	if l.Hooks == nil {
		return calls, nil
	}

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
		allowed = append(allowed, tc)
	}
	return allowed, blocked
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

	t, ok := tool.Get(tc.Name)
	if !ok {
		return message.ErrorResult(tc, fmt.Sprintf("Unknown tool: %s", tc.Name))
	}

	var toolResult ui.ToolResult
	if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
		toolResult = pat.ExecuteApproved(ctx, params, cwd)
	} else {
		toolResult = t.Execute(ctx, params, cwd)
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

// --- Compaction ---

// Compact summarizes a conversation to reduce context window usage.
// It sends the conversation to the LLM with a compact prompt and returns
// the summary text, the original message count, and any error.
func Compact(ctx context.Context, c *client.Client,
	msgs []message.Message, focus string) (summary string, count int, err error) {
	count = len(msgs)

	conversationText := message.BuildConversationText(msgs)

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

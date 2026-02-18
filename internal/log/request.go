package log

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// agentTrackerKey is the context key for AgentTurnTracker
type agentTrackerKey struct{}

// WithAgentTracker returns a context with the agent tracker attached
func WithAgentTracker(ctx context.Context, tracker *AgentTurnTracker) context.Context {
	return context.WithValue(ctx, agentTrackerKey{}, tracker)
}

// GetAgentTracker retrieves the agent tracker from context, or nil if not present
func GetAgentTracker(ctx context.Context) *AgentTurnTracker {
	if tracker, ok := ctx.Value(agentTrackerKey{}).(*AgentTurnTracker); ok {
		return tracker
	}
	return nil
}

// LogRequestCtx logs an LLM request with context (supports agent tracking)
func LogRequestCtx(ctx context.Context, providerName, model string, opts provider.CompletionOptions) {
	tracker := GetAgentTracker(ctx)
	var turn int
	var prefix string

	if tracker != nil {
		turn = tracker.NextTurn()
		prefix = tracker.GetTurnPrefix(turn)
		WriteAgentDevRequest(tracker, providerName, model, opts, turn)
	} else {
		turn = NextTurn()
		prefix = GetTurnPrefix(turn)
		WriteDevRequest(providerName, model, opts, turn)
	}

	if !enabled {
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "───────────────────────────────────────── %s ─────────────────────────────────────────\n", prefix)
	fmt.Fprintf(&sb, ">>> [%s] %s | max_tokens=%d temp=%.1f\n", providerName, model, opts.MaxTokens, opts.Temperature)

	if opts.SystemPrompt != "" {
		fmt.Fprintf(&sb, "    System: %s\n", escapeForLog(opts.SystemPrompt))
	}

	if len(opts.Tools) > 0 {
		toolNames := make([]string, len(opts.Tools))
		for i, t := range opts.Tools {
			toolNames[i] = t.Name
		}
		fmt.Fprintf(&sb, "    Tools(%d): [%s]\n", len(opts.Tools), strings.Join(toolNames, ", "))
	}

	fmt.Fprintf(&sb, "    Messages(%d):\n", len(opts.Messages))
	for i, msg := range opts.Messages {
		switch msg.Role {
		case message.RoleUser:
			if msg.Content != "" {
				fmt.Fprintf(&sb, "      [%d] User: %s\n", i, escapeForLog(msg.Content))
			}
			if msg.ToolResult != nil {
				if msg.ToolResult.IsError {
					fmt.Fprintf(&sb, "      [%d] ToolResult[%s] ERROR: %s\n", i, msg.ToolResult.ToolCallID, escapeForLog(msg.ToolResult.Content))
				} else {
					fmt.Fprintf(&sb, "      [%d] ToolResult[%s]: %s\n", i, msg.ToolResult.ToolCallID, escapeForLog(msg.ToolResult.Content))
				}
			}
		case message.RoleAssistant:
			if msg.Content != "" {
				fmt.Fprintf(&sb, "      [%d] Assistant: %s\n", i, escapeForLog(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "      [%d] ToolCall: %s(%s)\n", i, tc.Name, escapeForLog(tc.Input))
			}
		}
	}

	logger.Info(sb.String())
}

// LogRequest logs an LLM request in human-readable format (main loop only)
func LogRequest(providerName, model string, opts provider.CompletionOptions) {
	turn := NextTurn()

	// Write to DEV_DIR if enabled
	WriteDevRequest(providerName, model, opts, turn)

	if !enabled {
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "───────────────────────────────────────── Turn %d ─────────────────────────────────────────\n", turn)
	fmt.Fprintf(&sb, ">>> [%s] %s | max_tokens=%d temp=%.1f\n", providerName, model, opts.MaxTokens, opts.Temperature)

	if opts.SystemPrompt != "" {
		fmt.Fprintf(&sb, "    System: %s\n", escapeForLog(opts.SystemPrompt))
	}

	if len(opts.Tools) > 0 {
		toolNames := make([]string, len(opts.Tools))
		for i, t := range opts.Tools {
			toolNames[i] = t.Name
		}
		fmt.Fprintf(&sb, "    Tools(%d): [%s]\n", len(opts.Tools), strings.Join(toolNames, ", "))
	}

	fmt.Fprintf(&sb, "    Messages(%d):\n", len(opts.Messages))
	for i, msg := range opts.Messages {
		switch msg.Role {
		case message.RoleUser:
			if msg.Content != "" {
				fmt.Fprintf(&sb, "      [%d] User: %s\n", i, escapeForLog(msg.Content))
			}
			if msg.ToolResult != nil {
				if msg.ToolResult.IsError {
					fmt.Fprintf(&sb, "      [%d] ToolResult[%s] ERROR: %s\n", i, msg.ToolResult.ToolCallID, escapeForLog(msg.ToolResult.Content))
				} else {
					fmt.Fprintf(&sb, "      [%d] ToolResult[%s]: %s\n", i, msg.ToolResult.ToolCallID, escapeForLog(msg.ToolResult.Content))
				}
			}
		case message.RoleAssistant:
			if msg.Content != "" {
				fmt.Fprintf(&sb, "      [%d] Assistant: %s\n", i, escapeForLog(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "      [%d] ToolCall: %s(%s)\n", i, tc.Name, escapeForLog(tc.Input))
			}
		}
	}

	logger.Info(sb.String())
}

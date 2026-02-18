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
	} else {
		turn = NextTurn()
		prefix = GetTurnPrefix(turn)
	}

	writeDevRequest(tracker, providerName, model, opts, turn)
	logRequest(prefix, providerName, model, opts)
}

// LogRequest logs an LLM request in human-readable format (main loop only)
func LogRequest(providerName, model string, opts provider.CompletionOptions) {
	turn := NextTurn()
	prefix := fmt.Sprintf("Turn %d", turn)

	writeDevRequest(nil, providerName, model, opts, turn)
	logRequest(prefix, providerName, model, opts)
}

// logRequest formats and logs an LLM request.
func logRequest(prefix, providerName, model string, opts provider.CompletionOptions) {
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
		formatMessageLog(&sb, i, msg)
	}

	logger.Info(sb.String())
}

// formatMessageLog formats a single message for log output.
func formatMessageLog(sb *strings.Builder, idx int, msg message.Message) {
	switch msg.Role {
	case message.RoleUser:
		if msg.Content != "" {
			fmt.Fprintf(sb, "      [%d] User: %s\n", idx, escapeForLog(msg.Content))
		}
		if msg.ToolResult != nil {
			label := "ToolResult"
			if msg.ToolResult.IsError {
				label = "ToolResult ERROR"
			}
			fmt.Fprintf(sb, "      [%d] %s[%s]: %s\n", idx, label, msg.ToolResult.ToolCallID, escapeForLog(msg.ToolResult.Content))
		}
	case message.RoleAssistant:
		if msg.Content != "" {
			fmt.Fprintf(sb, "      [%d] Assistant: %s\n", idx, escapeForLog(msg.Content))
		}
		for _, tc := range msg.ToolCalls {
			fmt.Fprintf(sb, "      [%d] ToolCall: %s(%s)\n", idx, tc.Name, escapeForLog(tc.Input))
		}
	}
}

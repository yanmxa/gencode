package log

import (
	"context"
	"fmt"
	"strings"
)

// agentTrackerKey is the context key for AgentTurnTracker
type agentTrackerKey struct{}

// WithAgentTracker returns a context with the agent tracker attached
func WithAgentTracker(ctx context.Context, tracker *AgentTurnTracker) context.Context {
	return context.WithValue(ctx, agentTrackerKey{}, tracker)
}

// getAgentTracker retrieves the agent tracker from context, or nil if not present
func getAgentTracker(ctx context.Context) *AgentTurnTracker {
	if tracker, ok := ctx.Value(agentTrackerKey{}).(*AgentTurnTracker); ok {
		return tracker
	}
	return nil
}

// LogRequestCtx logs an LLM request with context (supports agent tracking).
// opts must satisfy the requestLoggable interface (duck-typed).
func LogRequestCtx(ctx context.Context, providerName, model string, opts any) {
	tracker := getAgentTracker(ctx)
	var turn int
	var prefix string

	if tracker != nil {
		turn = tracker.NextTurn()
		prefix = tracker.GetTurnPrefix(turn)
	} else {
		turn = nextTurn()
		prefix = getTurnPrefix(turn)
	}

	writeDevRequest(tracker, providerName, model, opts, turn)
	logRequest(prefix, providerName, model, opts)
}

// logRequest formats and logs an LLM request.
func logRequest(prefix, providerName, model string, opts any) {
	if !enabled {
		return
	}

	rl, ok := opts.(requestLoggable)
	if !ok {
		logger.Info(fmt.Sprintf(">>> [%s] %s (unstructured request)", providerName, model))
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "───────────────────────────────────────── %s ─────────────────────────────────────────\n", prefix)
	fmt.Fprintf(&sb, ">>> [%s] %s | max_tokens=%d temp=%.1f\n", providerName, model, rl.LogMaxTokens(), rl.LogTemperature())

	if sys := rl.LogSystemPrompt(); sys != "" {
		fmt.Fprintf(&sb, "    System: %s\n", EscapeForLog(sys))
	}

	if toolNames := rl.LogToolNames(); len(toolNames) > 0 {
		fmt.Fprintf(&sb, "    Tools(%d): [%s]\n", len(toolNames), strings.Join(toolNames, ", "))
	}

	fmt.Fprintf(&sb, "    Messages(%d):\n", rl.LogMessageCount())
	rl.LogFormatMessages(&sb)

	logger.Info(sb.String())
}

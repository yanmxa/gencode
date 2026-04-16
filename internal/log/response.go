package log

import (
	"context"
	"fmt"
	"strings"
)

// LogResponseCtx logs an LLM response with context (supports agent tracking).
// resp must satisfy the responseLoggable interface (duck-typed).
func LogResponseCtx(ctx context.Context, providerName string, resp any) {
	tracker := getAgentTracker(ctx)
	var turn int
	var prefix string

	if tracker != nil {
		turn = tracker.CurrentTurn()
		prefix = tracker.GetTurnPrefix(turn)
	} else {
		turn = currentTurn()
		prefix = getTurnPrefix(turn)
	}

	writeDevResponse(tracker, providerName, resp, turn)
	logResponse(prefix, providerName, resp)
}

// logResponse formats and logs an LLM response.
func logResponse(prefix, providerName string, resp any) {
	if !enabled {
		return
	}

	rl, ok := resp.(responseLoggable)
	if !ok {
		logger.Info(fmt.Sprintf("<<< [%s] %s (unstructured response)", prefix, providerName))
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<<< [%s] %s stop=%s | in=%d out=%d\n", prefix, providerName, rl.LogStopReason(), rl.LogInputTokens(), rl.LogOutputTokens())

	if content := rl.LogContent(); content != "" {
		sb.WriteString("    Content:\n")
		for _, line := range strings.Split(content, "\n") {
			fmt.Fprintf(&sb, "        %s\n", line)
		}
	}

	rl.LogFormatToolCalls(&sb)

	logger.Info(sb.String())
}

// LogError logs an error in human-readable format
func LogError(context string, err error) {
	if !enabled {
		return
	}
	logger.Error(fmt.Sprintf("!!! ERROR [%s] %v", context, err))
}

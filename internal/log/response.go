package log

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

// LogResponseCtx logs an LLM response with context (supports agent tracking)
func LogResponseCtx(ctx context.Context, providerName string, resp message.CompletionResponse) {
	tracker := GetAgentTracker(ctx)
	var turn int
	var prefix string

	if tracker != nil {
		turn = tracker.CurrentTurn()
		prefix = tracker.GetTurnPrefix(turn)
	} else {
		turn = CurrentTurn()
		prefix = GetTurnPrefix(turn)
	}

	writeDevResponse(tracker, providerName, resp, turn)
	logResponse(prefix, providerName, resp)
}

// LogResponse logs an LLM response in human-readable format (main loop only)
func LogResponse(providerName string, resp message.CompletionResponse) {
	turn := CurrentTurn()
	prefix := GetTurnPrefix(turn)

	writeDevResponse(nil, providerName, resp, turn)
	logResponse(prefix, providerName, resp)
}

// logResponse formats and logs an LLM response.
func logResponse(prefix, providerName string, resp message.CompletionResponse) {
	if !enabled {
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<<< [%s] %s stop=%s | in=%d out=%d\n", prefix, providerName, resp.StopReason, resp.Usage.InputTokens, resp.Usage.OutputTokens)

	if resp.Content != "" {
		sb.WriteString("    Content:\n")
		for _, line := range strings.Split(resp.Content, "\n") {
			fmt.Fprintf(&sb, "        %s\n", line)
		}
	}

	if len(resp.ToolCalls) > 0 {
		fmt.Fprintf(&sb, "    ToolCalls(%d):\n", len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			fmt.Fprintf(&sb, "      [%s] %s(%s)\n", tc.ID, tc.Name, escapeForLog(tc.Input))
		}
	}

	logger.Info(sb.String())
}

// LogError logs an error in human-readable format
func LogError(context string, err error) {
	if !enabled {
		return
	}
	logger.Error(fmt.Sprintf("!!! ERROR [%s] %v", context, err))
}

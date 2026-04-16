package log

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
)

// LogResponseCtx logs an LLM response with context (supports agent tracking).
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

	r, ok := resp.(core.CompletionResponse)
	if !ok {
		if rp, ok2 := resp.(*core.CompletionResponse); ok2 && rp != nil {
			r = *rp
		} else {
			logger.Info(fmt.Sprintf("<<< [%s] %s (unstructured response)", prefix, providerName))
			return
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<<< [%s] %s stop=%s | in=%d out=%d\n", prefix, providerName, r.StopReason, r.Usage.InputTokens, r.Usage.OutputTokens)

	if r.Content != "" {
		sb.WriteString("    Content:\n")
		for _, line := range strings.Split(r.Content, "\n") {
			fmt.Fprintf(&sb, "        %s\n", line)
		}
	}

	if len(r.ToolCalls) > 0 {
		fmt.Fprintf(&sb, "    ToolCalls(%d):\n", len(r.ToolCalls))
		for _, tc := range r.ToolCalls {
			fmt.Fprintf(&sb, "      [%s] %s(%s)\n", tc.ID, tc.Name, EscapeForLog(tc.Input))
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

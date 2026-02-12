package log

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/provider"
)

// LogResponse logs an LLM response in human-readable format
func LogResponse(providerName string, resp provider.CompletionResponse) {
	turn := CurrentTurn()

	// Write to DEV_DIR if enabled
	WriteDevResponse(providerName, resp, turn)

	if !enabled {
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<<< [%s] stop=%s | in=%d out=%d\n", providerName, resp.StopReason, resp.Usage.InputTokens, resp.Usage.OutputTokens)

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

	var sb strings.Builder
	fmt.Fprintf(&sb, "!!! ERROR [%s] %v\n", context, err)

	logger.Error(sb.String())
}

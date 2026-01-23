package log

import (
	"fmt"
	"strings"

	"github.com/myan/gencode/internal/provider"
)

// LogRequest logs an LLM request in human-readable format
func LogRequest(providerName, model string, opts provider.CompletionOptions) {
	if !enabled {
		return
	}

	turn := NextTurn()

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
		case "user":
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
		case "assistant":
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

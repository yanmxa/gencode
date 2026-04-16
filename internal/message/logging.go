package message

import (
	"fmt"
	"strings"
)

// The methods below satisfy the log package's responseLoggable and
// responseDevData interfaces through implicit duck typing.

// Compile-time assertions: ensure CompletionResponse satisfies the
// duck-typed logging interfaces. If a method is renamed or removed,
// these lines break at compile time rather than silently losing logging.
var _ interface{ LogStopReason() string } = CompletionResponse{}
var _ interface{ LogContent() string } = CompletionResponse{}
var _ interface{ LogThinking() string } = CompletionResponse{}
var _ interface{ LogInputTokens() int } = CompletionResponse{}
var _ interface{ LogOutputTokens() int } = CompletionResponse{}
var _ interface {
	LogFormatToolCalls(sb *strings.Builder)
} = CompletionResponse{}
var _ interface{ LogRawToolCalls() any } = CompletionResponse{}
var _ interface{ LogRawUsage() any } = CompletionResponse{}

// LogStopReason returns the stop reason for log output.
func (r CompletionResponse) LogStopReason() string { return r.StopReason }

// LogContent returns the response content for log output.
func (r CompletionResponse) LogContent() string { return r.Content }

// LogThinking returns the thinking content for log output.
func (r CompletionResponse) LogThinking() string { return r.Thinking }

// LogInputTokens returns the input token count for log output.
func (r CompletionResponse) LogInputTokens() int { return r.Usage.InputTokens }

// LogOutputTokens returns the output token count for log output.
func (r CompletionResponse) LogOutputTokens() int { return r.Usage.OutputTokens }

// LogFormatToolCalls writes a formatted summary of tool calls into sb.
func (r CompletionResponse) LogFormatToolCalls(sb *strings.Builder) {
	if len(r.ToolCalls) == 0 {
		return
	}
	fmt.Fprintf(sb, "    ToolCalls(%d):\n", len(r.ToolCalls))
	for _, tc := range r.ToolCalls {
		fmt.Fprintf(sb, "      [%s] %s(%s)\n", tc.ID, tc.Name, escapeForLog(tc.Input))
	}
}

// LogRawToolCalls returns the tool calls slice as any for JSON dev output.
func (r CompletionResponse) LogRawToolCalls() any { return r.ToolCalls }

// LogRawUsage returns the usage data as any for JSON dev output.
func (r CompletionResponse) LogRawUsage() any { return r.Usage }

// escapeForLog replaces newlines and tabs for single-line log output.
func escapeForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}


// Completion decision logic for the TUI-driven incremental loop.
// This is a copy of the logic from runtime/completion.go, moved here
// to eliminate the app → runtime dependency.
package app

import (
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

// completionAction describes what the TUI should do after receiving a completed response.
type completionAction int

const (
	completionEndTurn completionAction = iota
	completionRunTools
	completionRecoverMaxTokens
	completionStopMaxOutputRecovery
)

// completionDecision is the result of analyzing a completed LLM response.
type completionDecision struct {
	Action    completionAction
	ToolCalls []message.ToolCall
}

const (
	defaultMaxOutputRecovery = 3

	maxOutputRecoveryPrompt = "Your response was truncated due to output token limits. Resume directly from where you left off. Do not repeat any content."

	autoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."
)

// decideCompletion determines the next action after a completed assistant response.
func decideCompletion(stopReason string, calls []message.ToolCall, recoveryCount, maxRecovery int) completionDecision {
	if stopReason == "max_tokens" && len(calls) == 0 {
		if recoveryCount < maxRecovery {
			return completionDecision{Action: completionRecoverMaxTokens}
		}
		return completionDecision{Action: completionStopMaxOutputRecovery}
	}

	if len(calls) > 0 {
		return completionDecision{
			Action:    completionRunTools,
			ToolCalls: calls,
		}
	}

	return completionDecision{Action: completionEndTurn}
}

const minMessagesForCompaction = 3

// isPromptTooLong checks if an API error indicates the prompt exceeded the context window.
func isPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "prompt_too_long")
}

// canCompactMessages reports whether there is enough conversation to compact.
func canCompactMessages(messageCount int) bool {
	return messageCount >= minMessagesForCompaction
}

// shouldCompactPromptTooLong reports whether a prompt-too-long error should
// trigger a compaction attempt.
func shouldCompactPromptTooLong(err error, messageCount int) bool {
	return isPromptTooLong(err) && canCompactMessages(messageCount)
}

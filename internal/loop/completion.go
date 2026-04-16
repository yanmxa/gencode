package loop

import "github.com/yanmxa/gencode/internal/core"

// DecideCompletion determines the next action after a completed assistant response.
func DecideCompletion(stopReason string, calls []core.ToolCall, recoveryCount, maxRecovery int) CompletionDecision {
	if stopReason == "max_tokens" && len(calls) == 0 {
		if recoveryCount < maxRecovery {
			return CompletionDecision{Action: CompletionRecoverMaxTokens}
		}
		return CompletionDecision{Action: CompletionStopMaxOutputRecovery}
	}

	if len(calls) > 0 {
		return CompletionDecision{
			Action:    CompletionRunTools,
			ToolCalls: calls,
		}
	}

	return CompletionDecision{Action: CompletionEndTurn}
}

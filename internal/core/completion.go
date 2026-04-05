package core

import "github.com/yanmxa/gencode/internal/message"

// DecideCompletion determines the next action after a completed assistant response.
func DecideCompletion(stopReason string, calls []message.ToolCall, recoveryCount, maxRecovery int) CompletionDecision {
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

func (l *Loop) lastAssistantContent() string {
	return message.LastAssistantContent(l.messages)
}

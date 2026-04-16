package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/core/prompt"
)

// isPromptTooLong checks if an API error indicates the prompt exceeded the context window.
func isPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "prompt_too_long")
}

// CanCompactMessages reports whether there is enough conversation state to make
// a compaction worthwhile.
func CanCompactMessages(messageCount int) bool {
	return messageCount >= minMessagesForCompaction
}

// ShouldCompactPromptTooLong reports whether a prompt-too-long error should
// trigger a compaction attempt for the current conversation length.
func ShouldCompactPromptTooLong(err error, messageCount int) bool {
	return isPromptTooLong(err) && CanCompactMessages(messageCount)
}

// compactAndReplace summarizes the conversation and replaces messages with the summary.
// Returns true if compaction succeeded.
func (l *Loop) compactAndReplace(ctx context.Context, opts RunOptions) bool {
	summary, _, err := Compact(ctx, l.Client, l.messages, opts.SessionMemory, opts.CompactFocus)
	if err != nil {
		return false
	}
	l.messages = []message.Message{message.UserMessage(
		"Previous context summary:\n"+summary+"\n\nContinue with the task.", nil,
	)}
	return true
}

// Compact summarizes a conversation to reduce context window usage.
// It sends the conversation to the LLM with a compact prompt and returns
// the summary text, the original message count, and any error.
// sessionMemory is the previous compaction summary; if non-empty it is
// prepended so the new summary incorporates prior context.
func Compact(ctx context.Context, c *client.Client,
	msgs []message.Message, sessionMemory, focus string,
) (summary string, count int, err error) {
	count = len(msgs)

	conversationText := message.BuildConversationText(msgs)

	if sessionMemory != "" {
		conversationText = fmt.Sprintf("Previous session context:\n\n%s\n\n---\n\nRecent conversation:\n\n%s", sessionMemory, conversationText)
	}

	if focus != "" {
		conversationText += fmt.Sprintf("\n\n**Important**: Focus the summary on: %s", focus)
	}

	response, err := c.Complete(ctx,
		prompt.CompactPrompt(),
		[]message.Message{message.UserMessage(conversationText, nil)},
		2048,
	)
	if err != nil {
		return "", count, fmt.Errorf("failed to generate summary: %w", err)
	}

	summary = strings.TrimSpace(response.Content)
	if summary == "" {
		return "", count, fmt.Errorf("compaction produced empty summary")
	}

	return summary, count, nil
}

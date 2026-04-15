// Handler helpers for compact and token-limit result processing.
package compact

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/core/prompt"
)

// ShouldAutoCompact returns true when context usage is high enough to trigger
// automatic compaction.
func ShouldAutoCompact(llm provider.LLMProvider, messageCount, inputTokens int, store *provider.Store, currentModel *provider.CurrentModelInfo) bool {
	if llm == nil || messageCount < 3 {
		return false
	}
	return message.NeedsCompaction(inputTokens, GetEffectiveInputLimit(store, currentModel))
}

// GetContextUsagePercent returns what percentage of the context window is used.
func GetContextUsagePercent(inputTokens int, store *provider.Store, currentModel *provider.CurrentModelInfo) float64 {
	inputLimit := GetEffectiveInputLimit(store, currentModel)
	if inputLimit == 0 || inputTokens == 0 {
		return 0
	}
	return float64(inputTokens) / float64(inputLimit) * 100
}

// GetMaxTokens returns the effective output limit, falling back to defaultMaxTokens.
func GetMaxTokens(store *provider.Store, currentModel *provider.CurrentModelInfo, defaultMaxTokens int) int {
	if limit := GetEffectiveOutputLimit(store, currentModel); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

// CompactConversation compacts the message history into a summary.
// sessionMemory is the previous compaction summary loaded from persisted transcript state;
// if non-empty it is prepended as prior context so the new summary preserves it.
func CompactConversation(ctx context.Context, c *client.Client, msgs []message.Message, sessionMemory, focus string) (summary string, count int, err error) {
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

	return strings.TrimSpace(response.Content), count, nil
}

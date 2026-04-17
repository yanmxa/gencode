// Compact state, message types, and helper functions for conversation compaction
// and token-limit management.
package conv

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/llm"
)

// --- Message types ---

// CompactResultMsg is sent when a compaction operation completes.
type CompactResultMsg struct {
	Summary       string
	OriginalCount int
	Trigger       string // "manual" or "auto"
	Error         error
}

// TokenLimitResultMsg is sent when a token limit fetch completes.
type TokenLimitResultMsg struct {
	Result string
	Error  error
}

// --- Compact state ---

const PhaseSummarizing = "Summarizing conversation history"

// CompactState holds all compact-related state for the TUI model.
type CompactState struct {
	Active            bool
	Focus             string
	AutoContinue      bool
	LastResult        string
	LastError         bool
	Phase             string
	WarningSuppressed bool
}

// Reset clears all compact state.
func (c *CompactState) Reset() {
	c.Active = false
	c.Focus = ""
	c.AutoContinue = false
	c.LastResult = ""
	c.LastError = false
	c.Phase = ""
	c.WarningSuppressed = false
}

// ClearResult dismisses the last visible compact status.
func (c *CompactState) ClearResult() {
	c.LastResult = ""
	c.LastError = false
}

// Complete transitions compact state from running to a visible result state.
func (c *CompactState) Complete(result string, isError bool) {
	c.Active = false
	c.Focus = ""
	c.AutoContinue = false
	c.LastResult = result
	c.LastError = isError
	c.Phase = ""
	if !isError {
		c.WarningSuppressed = true
	}
}

// --- Update helpers ---

// ShouldAutoCompact returns true when context usage is high enough to trigger
// automatic compaction.
func ShouldAutoCompact(p llm.Provider, messageCount, inputTokens int, store *llm.Store, currentModel *llm.CurrentModelInfo) bool {
	if p == nil || messageCount < 3 {
		return false
	}
	return core.NeedsCompaction(inputTokens, GetEffectiveInputLimit(store, currentModel))
}

// GetContextUsagePercent returns what percentage of the context window is used.
func GetContextUsagePercent(inputTokens int, store *llm.Store, currentModel *llm.CurrentModelInfo) float64 {
	inputLimit := GetEffectiveInputLimit(store, currentModel)
	if inputLimit == 0 || inputTokens == 0 {
		return 0
	}
	return float64(inputTokens) / float64(inputLimit) * 100
}

// GetMaxTokens returns the effective output limit, falling back to defaultMaxTokens.
func GetMaxTokens(store *llm.Store, currentModel *llm.CurrentModelInfo, defaultMaxTokens int) int {
	if limit := getEffectiveOutputLimit(store, currentModel); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

// GetModelTokenLimits returns the cached token limits for the current model.
func GetModelTokenLimits(store *llm.Store, currentModel *llm.CurrentModelInfo) (inputLimit, outputLimit int) {
	if store == nil || currentModel == nil {
		return 0, 0
	}

	models, ok := store.GetCachedModels(currentModel.Provider, currentModel.AuthMethod)
	if !ok {
		return 0, 0
	}

	for _, model := range models {
		if model.ID == currentModel.ModelID {
			return model.InputTokenLimit, model.OutputTokenLimit
		}
	}
	return 0, 0
}

// getEffectiveTokenLimits returns custom limits if set, otherwise cached model limits.
func getEffectiveTokenLimits(store *llm.Store, currentModel *llm.CurrentModelInfo) (inputLimit, outputLimit int) {
	if currentModel == nil {
		return 0, 0
	}

	if store != nil {
		if input, output, ok := store.GetTokenLimit(currentModel.ModelID); ok {
			return input, output
		}
	}

	return GetModelTokenLimits(store, currentModel)
}

// GetEffectiveInputLimit returns only the effective input token limit.
func GetEffectiveInputLimit(store *llm.Store, currentModel *llm.CurrentModelInfo) int {
	input, _ := getEffectiveTokenLimits(store, currentModel)
	return input
}

// getEffectiveOutputLimit returns only the effective output token limit.
func getEffectiveOutputLimit(store *llm.Store, currentModel *llm.CurrentModelInfo) int {
	_, output := getEffectiveTokenLimits(store, currentModel)
	return output
}

// CompactConversation compacts the message history into a summary.
func CompactConversation(ctx context.Context, c *llm.Client, msgs []core.Message, sessionMemory, focus string) (summary string, count int, err error) {
	count = len(msgs)

	conversationText := core.BuildConversationText(msgs)

	if sessionMemory != "" {
		conversationText = fmt.Sprintf("Previous session context:\n\n%s\n\n---\n\nRecent conversation:\n\n%s", sessionMemory, conversationText)
	}

	if focus != "" {
		conversationText += fmt.Sprintf("\n\n**Important**: Focus the summary on: %s", focus)
	}

	response, err := c.Complete(ctx,
		system.CompactPrompt(),
		[]core.Message{core.UserMessage(conversationText, nil)},
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

func RenderCompactStatus(width int, spinnerView string, active bool, focus, phase, result string, isError bool) string {
	if !active && result == "" {
		return ""
	}

	label := "SESSION SUMMARY"
	title := "Conversation compacted"
	subtitle := "Older context was folded into a shorter summary. You can continue normally."
	detail := result
	accent := kit.CurrentTheme.Success
	icon := "✓"

	if active {
		if phase != "" {
			title = spinnerView + " " + phase
		} else {
			title = spinnerView + " Compacting conversation"
		}
		subtitle = "Summarizing recent history into a shorter reusable summary."
		if strings.TrimSpace(focus) != "" {
			detail = "Focus: " + focus
		} else {
			detail = "Preparing a smaller conversation state for the next turns."
		}
		accent = kit.CurrentTheme.Primary
		icon = ""
	} else if isError {
		label = "COMPACT ERROR"
		title = "Compact failed"
		subtitle = "Conversation history was not replaced. You can retry once the issue is resolved."
		accent = kit.CurrentTheme.Error
		icon = "✗"
	}

	if icon != "" {
		title = icon + " " + title
	}

	boxWidth := kit.CalculateBoxWidth(width)

	labelStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim).
		Bold(true)
	headerStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	subtitleStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.Text)
	bodyStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(kit.CurrentTheme.Background).
		Padding(0, 1).
		Width(boxWidth).
		MarginLeft(1)

	var lines []string
	lines = append(lines, labelStyle.Render(label))
	lines = append(lines, headerStyle.Render(title))
	if strings.TrimSpace(subtitle) != "" {
		lines = append(lines, subtitleStyle.Render(subtitle))
	}
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, bodyStyle.Render(detail))
	}

	return boxStyle.Render(strings.Join(lines, "\n"))
}

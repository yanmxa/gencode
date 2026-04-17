// Compact state, message types, and helper functions for conversation compaction
// and token-limit management.
package output

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/app/output/render"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/tool"
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

// CompactConversation compacts the message history into a summary.
// Delegates to runtime.Compact as the canonical implementation.
func CompactConversation(ctx context.Context, c *llm.Client, msgs []core.Message, sessionMemory, focus string) (summary string, count int, err error) {
	return runtime.Compact(ctx, c, msgs, sessionMemory, focus)
}

// --- Command helpers ---

// buildTokenLimitAgentPrompt returns the system prompt for the token-limit agent.
func buildTokenLimitAgentPrompt(modelID, providerName, authMethod string) string {
	return fmt.Sprintf(`You are a helpful assistant that finds token limits for AI models.

Your task is to find the maximum input tokens (context window) and maximum output tokens for this model:
- Model ID: %s
- Provider: %s
- Auth Method: %s

Use the WebSearch tool to search for this information, then use WebFetch to read relevant documentation pages if needed.

When you find the limits, respond with EXACTLY this format:
FOUND: <input_tokens> <output_tokens>

For example: FOUND: 200000 16000

If you cannot find the information after searching, respond with:
NOT_FOUND

Do not include any other text in your final response.`, modelID, providerName, authMethod)
}

// getTokenLimitAgentTools returns the tool definitions used by the token-limit agent.
func getTokenLimitAgentTools() []llm.ToolSchema {
	return []llm.ToolSchema{
		{
			Name:        "WebSearch",
			Description: "Search the web for information about model token limits",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "The search query"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "WebFetch",
			Description: "Fetch content from a URL to read documentation",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "The URL to fetch"},
				},
				"required": []string{"url"},
			},
		},
	}
}

// appendToolCallMessages executes tool calls and appends the results as messages.
func appendToolCallMessages(ctx context.Context, messages []core.Message, toolCalls []core.ToolCall, cwd string) []core.Message {
	messages = append(messages, core.AssistantMessage("", "", toolCalls))

	for _, tc := range toolCalls {
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			params = map[string]any{}
		}

		result := tool.Execute(ctx, tc.Name, params, cwd)
		messages = append(messages, core.ToolResultMessage(core.ToolResult{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    result.Output,
			IsError:    !result.Success,
		}))
	}
	return messages
}

// tokenLimitNotFoundMessage returns the user-facing message for unfound limits.
func tokenLimitNotFoundMessage(modelID string) string {
	return fmt.Sprintf("Could not find token limits for %s.\n\nSet manually with: /tokenlimit <input> <output>", modelID)
}

// parseTokenLimitResponse parses the agent response for FOUND/NOT_FOUND results.
// It returns the display string and whether processing is done.
// When limits are found and a store is provided, the limits are persisted.
func parseTokenLimitResponse(content, modelID string, store *llm.Store) (string, bool) {
	if strings.HasPrefix(content, "FOUND:") {
		var inputLimit, outputLimit int
		if _, err := fmt.Sscanf(content, "FOUND: %d %d", &inputLimit, &outputLimit); err == nil && inputLimit > 0 {
			if store != nil {
				_ = store.SetTokenLimit(modelID, inputLimit, outputLimit)
			}
			return fmt.Sprintf("Found and saved token limits for %s:\n  Input:  %s tokens\n  Output: %s tokens",
				modelID, render.FormatTokenCount(inputLimit), render.FormatTokenCount(outputLimit)), true
		}
	}

	if strings.Contains(content, "NOT_FOUND") {
		return tokenLimitNotFoundMessage(modelID), true
	}

	return "", false
}

// FormatTokenLimitDisplay formats a token limit display string.
func FormatTokenLimitDisplay(modelID string, inputLimit, outputLimit int, isCustom bool, currentInputTokens int) string {
	result := fmt.Sprintf("Token Limits for %s:\n\n  Input:  %s tokens\n  Output: %s tokens",
		modelID, render.FormatTokenCount(inputLimit), render.FormatTokenCount(outputLimit))

	if isCustom {
		result += "\n\n(custom override)"
	}

	if currentInputTokens > 0 && inputLimit > 0 {
		percent := float64(currentInputTokens) / float64(inputLimit) * 100
		result += fmt.Sprintf("\n\nCurrent usage: %s tokens (%.1f%%)", render.FormatTokenCount(currentInputTokens), percent)
	}

	return result
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

// AutoFetchTokenLimitsDeps holds the dependencies needed by AutoFetchTokenLimits.
type AutoFetchTokenLimitsDeps struct {
	LLM          llm.Provider
	Store        *llm.Store
	CurrentModel *llm.CurrentModelInfo
	ModelID      string
	Cwd          string
}

// AutoFetchTokenLimits fetches token limits for the current model.
// It first tries the provider's direct API (ModelLimitsFetcher) before
// falling back to a sub-agent discovery approach.
func AutoFetchTokenLimits(ctx context.Context, deps AutoFetchTokenLimitsDeps) (string, error) {
	if deps.LLM == nil {
		return "No provider connected. Use /tokenlimit <input> <output> to set manually.", nil
	}

	modelID := deps.CurrentModel.ModelID
	providerName := string(deps.CurrentModel.Provider)

	// Try direct API fetch if the provider supports it
	if fetcher, ok := deps.LLM.(llm.ModelLimitsFetcher); ok {
		inputLimit, outputLimit, err := fetcher.FetchModelLimits(ctx, modelID)
		if err == nil && (inputLimit > 0 || outputLimit > 0) {
			if deps.Store != nil {
				_ = deps.Store.SetTokenLimit(modelID, inputLimit, outputLimit)
			}
			return FormatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, 0), nil
		}
	}

	systemPrompt := buildTokenLimitAgentPrompt(modelID, providerName, string(deps.CurrentModel.AuthMethod))
	messages := []core.Message{
		core.UserMessage(fmt.Sprintf("Find the token limits for model: %s (provider: %s)", modelID, providerName), nil),
	}

	const maxTurns = 5

	for range maxTurns {
		response, err := llm.Complete(ctx, deps.LLM, llm.CompletionOptions{
			Model:        deps.ModelID,
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        getTokenLimitAgentTools(),
			MaxTokens:    1024,
		})
		if err != nil {
			return "", fmt.Errorf("agent error: %w", err)
		}

		if len(response.ToolCalls) > 0 {
			messages = appendToolCallMessages(ctx, messages, response.ToolCalls, deps.Cwd)
			continue
		}

		content := strings.TrimSpace(response.Content)
		if result, done := parseTokenLimitResponse(content, modelID, deps.Store); done {
			return result, nil
		}

		messages = append(messages,
			core.AssistantMessage(content, "", nil),
			core.UserMessage("Please continue searching or respond with FOUND or NOT_FOUND.", nil))
	}

	return tokenLimitNotFoundMessage(modelID), nil
}

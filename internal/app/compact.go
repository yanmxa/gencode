package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
)

// autoFetchTokenLimitsDeps holds the dependencies needed by autoFetchTokenLimits.
type autoFetchTokenLimitsDeps struct {
	LLM          llm.Provider
	Store        *llm.Store
	CurrentModel *llm.CurrentModelInfo
	ModelID      string
	Cwd          string
}

// autoFetchTokenLimits fetches token limits for the current model.
// It first tries the provider's direct API (ModelLimitsFetcher) before
// falling back to a sub-agent discovery approach.
func autoFetchTokenLimits(ctx context.Context, deps autoFetchTokenLimitsDeps) (string, error) {
	if deps.LLM == nil {
		return "No provider connected. Use /tokenlimit <input> <output> to set manually.", nil
	}

	modelID := deps.CurrentModel.ModelID
	providerName := string(deps.CurrentModel.Provider)

	if fetcher, ok := deps.LLM.(llm.ModelLimitsFetcher); ok {
		inputLimit, outputLimit, err := fetcher.FetchModelLimits(ctx, modelID)
		if err == nil && (inputLimit > 0 || outputLimit > 0) {
			if deps.Store != nil {
				_ = deps.Store.SetTokenLimit(modelID, inputLimit, outputLimit)
			}
			return formatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, 0), nil
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

func tokenLimitNotFoundMessage(modelID string) string {
	return fmt.Sprintf("Could not find token limits for %s.\n\nSet manually with: /tokenlimit <input> <output>", modelID)
}

func parseTokenLimitResponse(content, modelID string, store *llm.Store) (string, bool) {
	if strings.HasPrefix(content, "FOUND:") {
		var inputLimit, outputLimit int
		if _, err := fmt.Sscanf(content, "FOUND: %d %d", &inputLimit, &outputLimit); err == nil && inputLimit > 0 {
			if store != nil {
				_ = store.SetTokenLimit(modelID, inputLimit, outputLimit)
			}
			return fmt.Sprintf("Found and saved token limits for %s:\n  Input:  %s tokens\n  Output: %s tokens",
				modelID, formatTokenCount(inputLimit), formatTokenCount(outputLimit)), true
		}
	}

	if strings.Contains(content, "NOT_FOUND") {
		return tokenLimitNotFoundMessage(modelID), true
	}

	return "", false
}

func formatTokenLimitDisplay(modelID string, inputLimit, outputLimit int, isCustom bool, currentInputTokens int) string {
	result := fmt.Sprintf("Token Limits for %s:\n\n  Input:  %s tokens\n  Output: %s tokens",
		modelID, formatTokenCount(inputLimit), formatTokenCount(outputLimit))

	if isCustom {
		result += "\n\n(custom override)"
	}

	if currentInputTokens > 0 && inputLimit > 0 {
		percent := float64(currentInputTokens) / float64(inputLimit) * 100
		result += fmt.Sprintf("\n\nCurrent usage: %s tokens (%.1f%%)", formatTokenCount(currentInputTokens), percent)
	}

	return result
}

func formatTokenCount(count int) string {
	switch {
	case count >= 1000000:
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	case count >= 1000:
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	default:
		return fmt.Sprintf("%d", count)
	}
}

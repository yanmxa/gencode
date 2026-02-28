// Token limit and conversation compaction commands (/tokenlimit, /compact) with auto-fetch and auto-compact.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

type TokenLimitResultMsg struct {
	Result string
	Error  error
}

type CompactResultMsg struct {
	Summary       string
	OriginalCount int
	Error         error
}

func handleTokenLimitCommand(ctx context.Context, m *model, args string) (string, error) {
	if m.currentModel == nil {
		return "No model selected. Use /model to select a model first.", nil
	}

	modelID := m.currentModel.ModelID
	args = strings.TrimSpace(args)

	if args != "" {
		return setTokenLimits(m, modelID, args)
	}

	return showOrFetchTokenLimits(ctx, m, modelID)
}

func setTokenLimits(m *model, modelID, args string) (string, error) {
	var inputLimit, outputLimit int
	if _, err := fmt.Sscanf(args, "%d %d", &inputLimit, &outputLimit); err != nil {
		return "Usage:\n  /tokenlimit              - Show or auto-fetch limits\n  /tokenlimit <input> <output> - Set custom limits", nil
	}

	if inputLimit <= 0 || outputLimit <= 0 {
		return "Token limits must be positive integers", nil
	}

	if m.store != nil {
		if err := m.store.SetTokenLimit(modelID, inputLimit, outputLimit); err != nil {
			return "", fmt.Errorf("failed to set token limits: %w", err)
		}
	}

	return fmt.Sprintf("Set token limits for %s:\n  Input:  %s tokens\n  Output: %s tokens",
		modelID, formatTokenCount(inputLimit), formatTokenCount(outputLimit)), nil
}

func showOrFetchTokenLimits(ctx context.Context, m *model, modelID string) (string, error) {
	inputLimit, outputLimit := getModelTokenLimits(m)
	if inputLimit > 0 || outputLimit > 0 {
		if m.store != nil {
			if customInput, customOutput, ok := m.store.GetTokenLimit(modelID); ok {
				return formatTokenLimitDisplay(modelID, customInput, customOutput, true, m), nil
			}
		}
		return formatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, m), nil
	}

	m.fetchingTokenLimits = true
	return "", nil
}

func startTokenLimitFetch(m *model) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := autoFetchTokenLimits(ctx, m)
		return TokenLimitResultMsg{Result: result, Error: err}
	}
}

func formatTokenLimitDisplay(modelID string, inputLimit, outputLimit int, isCustom bool, m *model) string {
	result := fmt.Sprintf("Token Limits for %s:\n\n  Input:  %s tokens\n  Output: %s tokens",
		modelID, formatTokenCount(inputLimit), formatTokenCount(outputLimit))

	if isCustom {
		result += "\n\n(custom override)"
	}

	if m.lastInputTokens > 0 && inputLimit > 0 {
		percent := float64(m.lastInputTokens) / float64(inputLimit) * 100
		result += fmt.Sprintf("\n\nCurrent usage: %s tokens (%.1f%%)", formatTokenCount(m.lastInputTokens), percent)
	}

	return result
}

func autoFetchTokenLimits(ctx context.Context, m *model) (string, error) {
	if m.llmProvider == nil {
		return "No provider connected. Use /tokenlimit <input> <output> to set manually.", nil
	}

	modelID := m.currentModel.ModelID
	providerName := string(m.currentModel.Provider)

	systemPrompt := buildTokenLimitAgentPrompt(modelID, providerName, string(m.currentModel.AuthMethod))
	messages := []message.Message{
		message.UserMessage(fmt.Sprintf("Find the token limits for model: %s (provider: %s)", modelID, providerName), nil),
	}

	cwd := m.cwd
	const maxTurns = 5

	for range maxTurns {
		response, err := provider.Complete(ctx, m.llmProvider, provider.CompletionOptions{
			Model:        m.getModelID(),
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        getTokenLimitAgentTools(),
			MaxTokens:    1024,
		})
		if err != nil {
			return "", fmt.Errorf("agent error: %w", err)
		}

		if len(response.ToolCalls) > 0 {
			messages = appendToolCallMessages(ctx, messages, response.ToolCalls, cwd)
			continue
		}

		content := strings.TrimSpace(response.Content)
		if result, done := parseTokenLimitResponse(content, modelID, m); done {
			return result, nil
		}

		messages = append(messages,
			message.AssistantMessage(content, "", nil),
			message.UserMessage("Please continue searching or respond with FOUND or NOT_FOUND.", nil))
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

func getTokenLimitAgentTools() []provider.Tool {
	return []provider.Tool{
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

func appendToolCallMessages(ctx context.Context, messages []message.Message, toolCalls []message.ToolCall, cwd string) []message.Message {
	messages = append(messages, message.AssistantMessage("", "", toolCalls))

	for _, tc := range toolCalls {
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			params = map[string]any{}
		}

		result := tool.Execute(ctx, tc.Name, params, cwd)
		messages = append(messages, message.ToolResultMessage(message.ToolResult{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    result.Output,
			IsError:    !result.Success,
		}))
	}
	return messages
}

func parseTokenLimitResponse(content, modelID string, m *model) (string, bool) {
	if strings.HasPrefix(content, "FOUND:") {
		var inputLimit, outputLimit int
		if _, err := fmt.Sscanf(content, "FOUND: %d %d", &inputLimit, &outputLimit); err == nil && inputLimit > 0 {
			if m.store != nil {
				_ = m.store.SetTokenLimit(modelID, inputLimit, outputLimit)
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

func tokenLimitNotFoundMessage(modelID string) string {
	return fmt.Sprintf("Could not find token limits for %s.\n\nSet manually with: /tokenlimit <input> <output>", modelID)
}

func getModelTokenLimits(m *model) (inputLimit, outputLimit int) {
	if m.store == nil || m.currentModel == nil {
		return 0, 0
	}

	models, ok := m.store.GetCachedModels(m.currentModel.Provider, m.currentModel.AuthMethod)
	if !ok {
		return 0, 0
	}

	for _, model := range models {
		if model.ID == m.currentModel.ModelID {
			return model.InputTokenLimit, model.OutputTokenLimit
		}
	}
	return 0, 0
}

func (m *model) getEffectiveTokenLimits() (inputLimit, outputLimit int) {
	if m.currentModel == nil {
		return 0, 0
	}

	if m.store != nil {
		if input, output, ok := m.store.GetTokenLimit(m.currentModel.ModelID); ok {
			return input, output
		}
	}

	return getModelTokenLimits(m)
}

func (m *model) getEffectiveInputLimit() int {
	input, _ := m.getEffectiveTokenLimits()
	return input
}

func (m *model) getEffectiveOutputLimit() int {
	_, output := m.getEffectiveTokenLimits()
	return output
}

func (m *model) getMaxTokens() int {
	if limit := m.getEffectiveOutputLimit(); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

func handleCompactCommand(ctx context.Context, m *model, args string) (string, error) {
	if m.llmProvider == nil {
		return "No provider connected. Use /provider to connect.", nil
	}
	if len(m.messages) < 3 {
		return "Not enough conversation history to compact.", nil
	}
	if m.streaming {
		return "Cannot compact while streaming.", nil
	}
	m.compact.active = true
	m.compact.focus = strings.TrimSpace(args)
	return "", nil
}

func startCompact(m *model) tea.Cmd {
	focus := m.compact.focus
	return func() tea.Msg {
		ctx := context.Background()
		summary, count, err := compactConversation(ctx, m, focus)
		return CompactResultMsg{Summary: summary, OriginalCount: count, Error: err}
	}
}

func compactConversation(ctx context.Context, m *model, focus string) (summary string, count int, err error) {
	return core.Compact(ctx, m.loop.Client, m.convertMessagesToProvider(), focus)
}

func (m *model) getContextUsagePercent() float64 {
	inputLimit := m.getEffectiveInputLimit()
	if inputLimit == 0 || m.lastInputTokens == 0 {
		return 0
	}
	return float64(m.lastInputTokens) / float64(inputLimit) * 100
}

func (m *model) shouldAutoCompact() bool {
	if m.llmProvider == nil || len(m.messages) < 3 {
		return false
	}
	return message.NeedsCompaction(m.lastInputTokens, m.getEffectiveInputLimit())
}

func (m *model) triggerAutoCompact() tea.Cmd {
	m.compact.active = true
	m.compact.focus = ""
	m.messages = append(m.messages, chatMessage{
		role:    roleNotice,
		content: fmt.Sprintf("⚡ Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()),
	})
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.spinner.Tick, startCompact(m))
	return tea.Batch(commitCmds...)
}

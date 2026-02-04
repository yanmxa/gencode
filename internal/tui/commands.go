package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// TokenLimitResultMsg is sent when token limit fetching completes
type TokenLimitResultMsg struct {
	Result string
	Error  error
}

// CompactResultMsg is sent when conversation compaction completes
type CompactResultMsg struct {
	Summary       string
	OriginalCount int
	Error         error
}

// Command represents a slash command
type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

// CommandHandler is a function that handles a slash command
type CommandHandler func(ctx context.Context, m *model, args string) (string, error)

// getCommandRegistry returns the command registry
func getCommandRegistry() map[string]Command {
	return map[string]Command{
		"provider": {
			Name:        "provider",
			Description: "List and connect to LLM providers",
			Handler:     handleProviderCommand,
		},
		"model": {
			Name:        "model",
			Description: "List and select models",
			Handler:     handleModelCommand,
		},
		"clear": {
			Name:        "clear",
			Description: "Clear chat history",
			Handler:     handleClearCommand,
		},
		"help": {
			Name:        "help",
			Description: "Show available commands",
			Handler:     handleHelpCommand,
		},
		"glob": {
			Name:        "glob",
			Description: "Find files matching a pattern",
			Handler:     handleGlobCommand,
		},
		"tools": {
			Name:        "tools",
			Description: "Manage available tools (enable/disable)",
			Handler:     handleToolCommand,
		},
		"plan": {
			Name:        "plan",
			Description: "Enter plan mode to explore and plan before execution",
			Handler:     handlePlanCommand,
		},
		"skills": {
			Name:        "skills",
			Description: "Manage skills (enable/disable/activate)",
			Handler:     handleSkillCommand,
		},
		"agents": {
			Name:        "agents",
			Description: "Manage available agents (enable/disable)",
			Handler:     handleAgentCommand,
		},
		"tokenlimit": {
			Name:        "tokenlimit",
			Description: "View or set token limits for current model",
			Handler:     handleTokenLimitCommand,
		},
		"compact": {
			Name:        "compact",
			Description: "Summarize conversation to reduce context size",
			Handler:     handleCompactCommand,
		},
		"sessions": {
			Name:        "sessions",
			Description: "List and resume previous sessions",
			Handler:     handleSessionsCommand,
		},
		"save": {
			Name:        "save",
			Description: "Manually save the current session",
			Handler:     handleSaveCommand,
		},
		"init": {
			Name:        "init",
			Description: "Initialize project memory file (GEN.md)",
			Handler:     handleInitCommand,
		},
		"memory": {
			Name:        "memory",
			Description: "View and manage memory files (list/show/edit)",
			Handler:     handleMemoryCommand,
		},
	}
}

// ParseCommand parses input and returns command name and args if it's a slash command
func ParseCommand(input string) (cmd string, args string, isCmd bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)
	cmd = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return cmd, args, true
}

// ExecuteCommand executes a slash command
func ExecuteCommand(ctx context.Context, m *model, input string) (string, bool) {
	cmd, args, isCmd := ParseCommand(input)
	if !isCmd {
		return "", false
	}

	// First check built-in commands
	registry := getCommandRegistry()
	command, ok := registry[cmd]
	if ok {
		result, err := command.Handler(ctx, m, args)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		return result, true
	}

	// Then check skill commands
	if sk, ok := IsSkillCommand(cmd); ok {
		return executeSkillCommand(m, sk, args), true
	}

	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd), true
}

// executeSkillCommand executes a skill command by loading its instructions
// and preparing them for the next LLM request.
func executeSkillCommand(m *model, sk *skill.Skill, args string) string {
	// Load full skill instructions for the next prompt (using FullName)
	if skill.DefaultRegistry != nil {
		m.pendingSkillInstructions = skill.DefaultRegistry.GetSkillInvocationPrompt(sk.FullName())
	}

	// Prepare user message - keep it clean and simple
	if args != "" {
		// User provided arguments, use them directly
		m.pendingSkillArgs = args
	} else {
		// No arguments - just invoke the skill
		m.pendingSkillArgs = fmt.Sprintf("Run /%s", sk.FullName())
	}

	return "" // Return empty to trigger LLM call with skill context
}

// GetMatchingCommands returns commands matching the query using fuzzy search
func GetMatchingCommands(query string) []Command {
	query = strings.ToLower(strings.TrimPrefix(query, "/"))
	matches := make([]Command, 0)

	// Add matching built-in commands
	registry := getCommandRegistry()
	for name, cmd := range registry {
		if fuzzyMatch(name, query) {
			matches = append(matches, cmd)
		}
	}

	// Add matching skill commands
	skillCmds := GetSkillCommands()
	for _, cmd := range skillCmds {
		if fuzzyMatch(strings.ToLower(cmd.Name), query) {
			// Avoid duplicates with built-in commands
			if _, exists := registry[cmd.Name]; !exists {
				matches = append(matches, cmd)
			}
		}
	}

	// Sort by name
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// handleProviderCommand handles the /provider command
func handleProviderCommand(ctx context.Context, m *model, args string) (string, error) {
	// Enter interactive selection mode
	if err := m.selector.EnterProviderSelect(m.width, m.height); err != nil {
		return "", err
	}

	// Return empty string - the selection UI will be shown
	return "", nil
}

// handleModelCommand handles the /model command
func handleModelCommand(ctx context.Context, m *model, args string) (string, error) {
	// Enter interactive selection mode
	if err := m.selector.EnterModelSelect(ctx, m.width, m.height); err != nil {
		return "", err
	}

	// Return empty string - the selection UI will be shown
	return "", nil
}

// handleHelpCommand handles the /help command
func handleHelpCommand(ctx context.Context, m *model, args string) (string, error) {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	registry := getCommandRegistry()

	// Sort commands by name
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmd := registry[name]
		sb.WriteString(fmt.Sprintf("  /%s - %s\n", cmd.Name, cmd.Description))
	}

	return sb.String(), nil
}

// handleClearCommand handles the /clear command
func handleClearCommand(ctx context.Context, m *model, args string) (string, error) {
	// Clear all messages and reset token tracking
	m.messages = []chatMessage{}
	m.lastInputTokens = 0
	m.lastOutputTokens = 0
	return "", nil
}

// handleGlobCommand handles the /glob command
func handleGlobCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"pattern": args}

	// Check if a path is specified
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "glob", params, cwd)
	return ui.RenderToolResult(result, m.width), nil
}

// handleToolCommand handles the /tool command
func handleToolCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.toolSelector.EnterToolSelect(m.width, m.height, m.disabledTools); err != nil {
		return "", err
	}
	return "", nil
}

// handlePlanCommand handles the /plan command
func handlePlanCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil
	}

	m.operationMode = modePlan
	m.planMode = true
	m.planTask = args

	// Reset permissions (sync with mode)
	m.sessionPermissions.AllowAllEdits = false
	m.sessionPermissions.AllowAllWrites = false
	m.sessionPermissions.AllowAllBash = false
	m.sessionPermissions.AllowAllSkills = false

	store, err := plan.NewStore()
	if err != nil {
		return "", fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.planStore = store

	return fmt.Sprintf("Entering plan mode for: %s\n\nI will explore the codebase and create an implementation plan. Only read-only tools are available until the plan is approved.", args), nil
}

// handleSkillCommand handles the /skills command
func handleSkillCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.skillSelector.EnterSkillSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

// handleAgentCommand handles the /agent and /agents commands
func handleAgentCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.agentSelector.EnterAgentSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

// handleTokenLimitCommand handles the /tokenlimit command
func handleTokenLimitCommand(ctx context.Context, m *model, args string) (string, error) {
	if m.currentModel == nil {
		return "No model selected. Use /model to select a model first.", nil
	}

	modelID := m.currentModel.ModelID
	args = strings.TrimSpace(args)

	// Set custom limits: /tokenlimit <input> <output>
	if args != "" {
		return setTokenLimits(m, modelID, args)
	}

	// Show existing limits or auto-fetch
	return showOrFetchTokenLimits(ctx, m, modelID)
}

// setTokenLimits parses and saves custom token limits
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

// showOrFetchTokenLimits displays existing limits or starts async auto-fetch
func showOrFetchTokenLimits(ctx context.Context, m *model, modelID string) (string, error) {
	// Check model cache (built-in limits from provider)
	inputLimit, outputLimit := getModelTokenLimits(m)
	if inputLimit > 0 || outputLimit > 0 {
		// Check if there's a custom override to display instead
		if m.store != nil {
			if customInput, customOutput, ok := m.store.GetTokenLimit(modelID); ok {
				return formatTokenLimitDisplay(modelID, customInput, customOutput, true, m), nil
			}
		}
		return formatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, m), nil
	}

	// Model cache has no limits - start async auto-fetch
	m.fetchingTokenLimits = true
	return "", nil // Empty result triggers async fetch
}

// startTokenLimitFetch returns a tea.Cmd that fetches token limits in background
func startTokenLimitFetch(m *model) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := autoFetchTokenLimits(ctx, m)
		return TokenLimitResultMsg{Result: result, Error: err}
	}
}

// formatTokenLimitDisplay formats token limits for display
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

// autoFetchTokenLimits uses an agent loop to search and extract token limits
func autoFetchTokenLimits(ctx context.Context, m *model) (string, error) {
	if m.llmProvider == nil {
		return "No provider connected. Use /tokenlimit <input> <output> to set manually.", nil
	}

	modelID := m.currentModel.ModelID
	providerName := string(m.currentModel.Provider)

	systemPrompt := buildTokenLimitAgentPrompt(modelID, providerName, string(m.currentModel.AuthMethod))
	messages := []provider.Message{
		{Role: "user", Content: fmt.Sprintf("Find the token limits for model: %s (provider: %s)", modelID, providerName)},
	}

	cwd, _ := os.Getwd()
	const maxTurns = 5

	for turn := 0; turn < maxTurns; turn++ {
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

		// Execute tool calls if present
		if len(response.ToolCalls) > 0 {
			messages = appendToolCallMessages(ctx, messages, response.ToolCalls, cwd)
			continue
		}

		// Parse final response
		content := strings.TrimSpace(response.Content)
		if result, done := parseTokenLimitResponse(content, modelID, m); done {
			return result, nil
		}

		// Continue conversation
		messages = append(messages,
			provider.Message{Role: "assistant", Content: content},
			provider.Message{Role: "user", Content: "Please continue searching or respond with FOUND or NOT_FOUND."})
	}

	return tokenLimitNotFoundMessage(modelID), nil
}

// buildTokenLimitAgentPrompt creates the system prompt for the token limit agent
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

// getTokenLimitAgentTools returns the tools available to the token limit agent
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

// appendToolCallMessages executes tool calls and appends results to messages
func appendToolCallMessages(ctx context.Context, messages []provider.Message, toolCalls []provider.ToolCall, cwd string) []provider.Message {
	messages = append(messages, provider.Message{
		Role:      "assistant",
		ToolCalls: toolCalls,
	})

	for _, tc := range toolCalls {
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			params = map[string]any{}
		}

		result := tool.Execute(ctx, tc.Name, params, cwd)
		messages = append(messages, provider.Message{
			Role: "user",
			ToolResult: &provider.ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    result.Output,
				IsError:    !result.Success,
			},
		})
	}
	return messages
}

// parseTokenLimitResponse parses the agent response and saves limits if found
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

// tokenLimitNotFoundMessage returns the standard not-found message
func tokenLimitNotFoundMessage(modelID string) string {
	return fmt.Sprintf("Could not find token limits for %s.\n\nSet manually with: /tokenlimit <input> <output>", modelID)
}

// getModelTokenLimits returns token limits from model cache
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

// getEffectiveInputLimit returns the effective input token limit for the current model
// Priority: 1. Custom override, 2. Model cache, 3. Return 0 (no limit)
func (m *model) getEffectiveInputLimit() int {
	if m.currentModel == nil {
		return 0
	}

	// Check custom override first
	if m.store != nil {
		if inputLimit, _, ok := m.store.GetTokenLimit(m.currentModel.ModelID); ok {
			return inputLimit
		}
	}

	// Check model cache
	inputLimit, _ := getModelTokenLimits(m)
	return inputLimit
}

// getEffectiveOutputLimit returns the effective output token limit for the current model
// Priority: 1. Custom override, 2. Model cache, 3. Return 0 (no limit)
func (m *model) getEffectiveOutputLimit() int {
	if m.currentModel == nil {
		return 0
	}

	// Check custom override first
	if m.store != nil {
		if _, outputLimit, ok := m.store.GetTokenLimit(m.currentModel.ModelID); ok {
			return outputLimit
		}
	}

	// Check model cache
	_, outputLimit := getModelTokenLimits(m)
	return outputLimit
}

// getMaxTokens returns the max tokens to use for API requests
// Uses effective output limit if available, otherwise falls back to default
func (m *model) getMaxTokens() int {
	if limit := m.getEffectiveOutputLimit(); limit > 0 {
		return limit
	}
	return defaultMaxTokens
}

// formatTokenCount formats a token count for display (e.g., 200000 -> "200K")
func formatTokenCount(count int) string {
	if count >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	if count >= 1000 {
		return fmt.Sprintf("%dK", count/1000)
	}
	return fmt.Sprintf("%d", count)
}

// handleCompactCommand handles the /compact command
// Usage: /compact [focus] - optionally specify what to focus on in the summary
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
	m.compacting = true
	m.compactFocus = strings.TrimSpace(args) // Store optional focus
	return "", nil
}

// startCompact returns a tea.Cmd that compacts the conversation in background
func startCompact(m *model) tea.Cmd {
	focus := m.compactFocus // Capture focus before async execution
	return func() tea.Msg {
		ctx := context.Background()
		summary, count, err := compactConversation(ctx, m, focus)
		return CompactResultMsg{Summary: summary, OriginalCount: count, Error: err}
	}
}

// compactConversation calls the LLM to generate a summary of the conversation
func compactConversation(ctx context.Context, m *model, focus string) (summary string, count int, err error) {
	count = len(m.messages)
	conversationText := buildConversationForSummary(m.messages)

	// Add focus instruction if provided
	if focus != "" {
		conversationText += fmt.Sprintf("\n\n**Important**: Focus the summary on: %s", focus)
	}

	response, err := provider.Complete(ctx, m.llmProvider, provider.CompletionOptions{
		Model:        m.getModelID(),
		SystemPrompt: system.CompactPrompt(),
		Messages:     []provider.Message{{Role: "user", Content: conversationText}},
		MaxTokens:    2048,
	})
	if err != nil {
		return "", count, fmt.Errorf("failed to generate summary: %w", err)
	}

	return strings.TrimSpace(response.Content), count, nil
}

// buildConversationForSummary converts messages to text for summarization
func buildConversationForSummary(messages []chatMessage) string {
	var sb strings.Builder
	sb.WriteString("Please summarize this coding conversation:\n\n")

	for _, msg := range messages {
		switch msg.role {
		case "user":
			if msg.toolResult != nil {
				content := truncateToolResult(msg.toolResult.Content, 500)
				fmt.Fprintf(&sb, "[Tool Result: %s]\n%s\n\n", msg.toolName, content)
			} else {
				fmt.Fprintf(&sb, "User: %s\n\n", msg.content)
			}

		case "assistant":
			if msg.content != "" {
				fmt.Fprintf(&sb, "Assistant: %s\n\n", msg.content)
			}
			if len(msg.toolCalls) > 0 {
				for _, tc := range msg.toolCalls {
					fmt.Fprintf(&sb, "[Tool Call: %s]\n", tc.Name)
				}
				sb.WriteString("\n")
			}

		case "system":
			// Skip welcome messages
			if msg.content != "" && !strings.Contains(msg.content, "AI-powered coding assistant") {
				fmt.Fprintf(&sb, "System: %s\n\n", msg.content)
			}
		}
	}

	return sb.String()
}

// truncateToolResult truncates tool result content to maxLen characters
func truncateToolResult(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "...[truncated]"
}

// getContextUsagePercent returns the current context usage as a percentage.
// Returns 0 if limits are not available.
func (m *model) getContextUsagePercent() float64 {
	inputLimit := m.getEffectiveInputLimit()
	if inputLimit == 0 || m.lastInputTokens == 0 {
		return 0
	}
	return float64(m.lastInputTokens) / float64(inputLimit) * 100
}

// shouldAutoCompact checks if auto-compact should be triggered
func (m *model) shouldAutoCompact() bool {
	if m.llmProvider == nil || len(m.messages) < 3 {
		return false
	}
	return m.getContextUsagePercent() >= autoCompactThreshold
}

// triggerAutoCompact initiates auto-compact with a system message
func (m *model) triggerAutoCompact() tea.Cmd {
	m.compacting = true
	m.compactFocus = ""
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: fmt.Sprintf("⚡ Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()),
	})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return tea.Batch(m.spinner.Tick, startCompact(m))
}

// IsSkillCommand checks if the command is a registered skill.
// Returns the skill and true if found, nil and false otherwise.
func IsSkillCommand(cmd string) (*skill.Skill, bool) {
	if skill.DefaultRegistry == nil {
		return nil, false
	}

	s, ok := skill.DefaultRegistry.Get(cmd)
	if !ok {
		return nil, false
	}

	// Only return enabled or active skills as commands
	if !s.IsEnabled() {
		return nil, false
	}

	return s, true
}

// GetSkillCommands returns skill commands for command suggestions.
// Skill names use the format namespace:name (e.g., git:commit).
func GetSkillCommands() []Command {
	if skill.DefaultRegistry == nil {
		return nil
	}

	var cmds []Command
	for _, s := range skill.DefaultRegistry.GetEnabled() {
		hint := ""
		if s.ArgumentHint != "" {
			hint = " " + s.ArgumentHint
		}
		// Use FullName (namespace:name) as command name
		cmds = append(cmds, Command{
			Name:        s.FullName(),
			Description: s.Description + hint,
		})
	}
	return cmds
}

// handleSessionsCommand handles the /sessions command
func handleSessionsCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.ensureSessionStore(); err != nil {
		return "", fmt.Errorf("failed to initialize session store: %w", err)
	}

	if err := m.sessionSelector.EnterSessionSelect(m.width, m.height, m.sessionStore); err != nil {
		return "", err
	}

	return "", nil
}

// handleSaveCommand handles the /save command
func handleSaveCommand(ctx context.Context, m *model, args string) (string, error) {
	if len(m.messages) == 0 {
		return "No messages to save.", nil
	}

	if err := m.saveSession(); err != nil {
		return "", fmt.Errorf("failed to save session: %w", err)
	}

	return fmt.Sprintf("Session saved: %s", m.currentSessionID), nil
}

// handleInitCommand handles the /init command
// Usage: /init [--claude]
func handleInitCommand(ctx context.Context, m *model, args string) (string, error) {
	isClaude := strings.Contains(args, "--claude")

	// Determine file paths based on format
	var targetDir, fileName string
	if isClaude {
		targetDir = filepath.Join(m.cwd, ".claude")
		fileName = "CLAUDE.md"
	} else {
		targetDir = filepath.Join(m.cwd, ".gen")
		fileName = "GEN.md"
	}
	filePath := filepath.Join(targetDir, fileName)

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit to modify it.", filePath), nil
	}

	// Create directory and write file
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getProjectTemplate(m.cwd)), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return fmt.Sprintf("Created %s\n\nEdit with: /memory edit", filePath), nil
}

// handleMemoryCommand handles the /memory command
// Usage: /memory [list|show|edit] [user|project]
func handleMemoryCommand(ctx context.Context, m *model, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	subCmd := "list"
	if len(parts) > 0 {
		subCmd = strings.ToLower(parts[0])
	}

	scope := "project"
	if len(parts) > 1 {
		scope = strings.ToLower(parts[1])
	}

	switch subCmd {
	case "list", "":
		return handleMemoryList(m)
	case "show":
		return handleMemoryShow(m)
	case "edit":
		return handleMemoryEdit(m, scope)
	default:
		return "Usage: /memory [list|show|edit] [user|project]", nil
	}
}

// handleMemoryList lists all loaded memory files
func handleMemoryList(m *model) (string, error) {
	userPaths, projectPaths := system.GetMemoryPaths(m.cwd)

	var sb strings.Builder
	sb.WriteString("Memory Files:\n\n")

	// Helper to format a memory level section
	formatLevel := func(label string, paths []string, createHint string) {
		sb.WriteString(label + ":\n")
		if found := system.FindMemoryFile(paths); found != "" {
			fmt.Fprintf(&sb, "  + %s\n", found)
		} else {
			sb.WriteString("  (none)\n")
			fmt.Fprintf(&sb, "  Create with: %s\n", createHint)
		}
	}

	formatLevel("User level", userPaths, "/init --claude or manually at "+userPaths[0])
	sb.WriteString("\n")
	formatLevel("Project level", projectPaths, "/init")

	return sb.String(), nil
}

// handleMemoryShow displays current memory content
func handleMemoryShow(m *model) (string, error) {
	content := system.LoadMemory(m.cwd)
	if content == "" {
		return "No memory files loaded.\n\nCreate project memory with: /init", nil
	}

	// Truncate if too long
	const maxShow = 2000
	if len(content) > maxShow {
		content = content[:maxShow] + "\n\n... (truncated)"
	}

	return fmt.Sprintf("Current Memory:\n\n%s", content), nil
}

// handleMemoryEdit opens memory file in editor
func handleMemoryEdit(m *model, scope string) (string, error) {
	userPaths, projectPaths := system.GetMemoryPaths(m.cwd)

	var filePath string
	if scope == "user" {
		filePath = system.FindMemoryFile(userPaths)
		if filePath == "" {
			// Create default user memory file
			filePath = userPaths[0]
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return "", fmt.Errorf("failed to create directory: %w", err)
			}
			if err := os.WriteFile(filePath, []byte(getUserTemplate()), 0644); err != nil {
				return "", fmt.Errorf("failed to create file: %w", err)
			}
		}
	} else {
		filePath = system.FindMemoryFile(projectPaths)
		if filePath == "" {
			return "No project memory file found.\n\nCreate with: /init", nil
		}
	}

	m.editingMemoryFile = filePath
	return "", nil // Signal to start editor
}

// getProjectTemplate returns the default project memory template
func getProjectTemplate(cwd string) string {
	projectName := filepath.Base(cwd)
	return fmt.Sprintf(`# GEN.md

This file provides guidance to GenCode when working with code in this repository.

## Project Overview

%s - Describe what this project does.

## Build & Run

`+"```bash"+`
# Add your build commands here
`+"```"+`

## Architecture

<!-- Key directories and their purpose -->

## Key Patterns

<!-- Important conventions to follow -->
`, projectName)
}

// getUserTemplate returns the default user memory template
func getUserTemplate() string {
	return `# GEN.md

User-level instructions for GenCode.

## Coding Preferences

<!-- Your preferred coding style -->

## Security

<!-- Security practices to follow -->
`
}

// startExternalEditor returns a tea.Cmd that launches an external editor
func startExternalEditor(filePath string) tea.Cmd {
	editor := getEditor()
	cmd := exec.Command(editor, filePath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

// getEditor returns the user's preferred editor from environment or a fallback
func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	for _, e := range []string{"vim", "nano", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return "vi" // Last resort fallback
}

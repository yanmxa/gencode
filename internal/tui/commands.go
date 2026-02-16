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
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
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
		"init": {
			Name:        "init",
			Description: "Initialize memory files (GEN.md, local, rules)",
			Handler:     handleInitCommand,
		},
		"memory": {
			Name:        "memory",
			Description: "View and manage memory files (list/show/edit) with @import support",
			Handler:     handleMemoryCommand,
		},
		"mcp": {
			Name:        "mcp",
			Description: "Manage MCP servers (add/remove/connect/list)",
			Handler:     handleMCPCommand,
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
	// Clear all messages, reset token tracking, and mark for screen clear
	m.messages = []chatMessage{}
	m.committedCount = 0
	m.lastInputTokens = 0
	m.lastOutputTokens = 0
	m.pendingClearScreen = true
	// Reset task list for fresh session
	tool.DefaultTodoStore.Reset()
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
	var mcpTools func() []provider.Tool
	if m.mcpRegistry != nil {
		mcpTools = m.mcpRegistry.GetToolSchemas
	}
	if err := m.toolSelector.EnterToolSelect(m.width, m.height, m.disabledTools, mcpTools); err != nil {
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
	messages := []message.Message{
		message.UserMessage(fmt.Sprintf("Find the token limits for model: %s (provider: %s)", modelID, providerName), nil),
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
			message.AssistantMessage(content, "", nil),
			message.UserMessage("Please continue searching or respond with FOUND or NOT_FOUND.", nil))
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

// getEffectiveTokenLimits returns effective input and output token limits.
// Priority: 1. Custom override, 2. Model cache, 3. Return 0 (no limit)
func (m *model) getEffectiveTokenLimits() (inputLimit, outputLimit int) {
	if m.currentModel == nil {
		return 0, 0
	}

	// Check custom override first
	if m.store != nil {
		if input, output, ok := m.store.GetTokenLimit(m.currentModel.ModelID); ok {
			return input, output
		}
	}

	// Fall back to model cache
	return getModelTokenLimits(m)
}

// getEffectiveInputLimit returns the effective input token limit for the current model.
func (m *model) getEffectiveInputLimit() int {
	input, _ := m.getEffectiveTokenLimits()
	return input
}

// getEffectiveOutputLimit returns the effective output token limit for the current model.
func (m *model) getEffectiveOutputLimit() int {
	_, output := m.getEffectiveTokenLimits()
	return output
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
	return core.Compact(ctx, m.loop.Client, m.convertMessagesToProvider(), focus)
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
	return message.NeedsCompaction(m.lastInputTokens, m.getEffectiveInputLimit())
}

// triggerAutoCompact initiates auto-compact with a system message
func (m *model) triggerAutoCompact() tea.Cmd {
	m.compacting = true
	m.compactFocus = ""
	m.messages = append(m.messages, chatMessage{
		role:    roleNotice,
		content: fmt.Sprintf("⚡ Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()),
	})
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.spinner.Tick, startCompact(m))
	return tea.Batch(commitCmds...)
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

// handleInitCommand handles the /init command
// Usage: /init [local|rules] [--claude]
func handleInitCommand(ctx context.Context, m *model, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	isClaude := strings.Contains(args, "--claude")

	// Parse subcommand
	subCmd := ""
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "--") {
		subCmd = strings.ToLower(parts[0])
	}

	switch subCmd {
	case "local":
		return handleInitLocal(m)
	case "rules":
		return handleInitRules(m, isClaude)
	default:
		return handleInitProject(m, isClaude)
	}
}

// handleInitProject creates the main project memory file
func handleInitProject(m *model, isClaude bool) (string, error) {
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

// handleInitLocal creates the local memory file (not committed to git)
func handleInitLocal(m *model) (string, error) {
	targetDir := filepath.Join(m.cwd, ".gen")
	filePath := filepath.Join(targetDir, "GEN.local.md")

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit local to modify it.", filePath), nil
	}

	// Create directory and write file
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getLocalTemplate()), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	// Add to .gitignore if it exists
	addToGitignore(m.cwd, "GEN.local.md")

	return fmt.Sprintf("Created %s (added to .gitignore)\n\nEdit with: /memory edit local", filePath), nil
}

// handleInitRules creates the rules directory structure
func handleInitRules(m *model, isClaude bool) (string, error) {
	var rulesDir string
	if isClaude {
		rulesDir = filepath.Join(m.cwd, ".claude", "rules")
	} else {
		rulesDir = filepath.Join(m.cwd, ".gen", "rules")
	}

	// Check if directory already exists
	if _, err := os.Stat(rulesDir); err == nil {
		return fmt.Sprintf("Directory already exists: %s", rulesDir), nil
	}

	// Create directory
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", rulesDir, err)
	}

	// Create example rule file
	examplePath := filepath.Join(rulesDir, "example.md")
	if err := os.WriteFile(examplePath, []byte(getRulesTemplate()), 0644); err != nil {
		return "", fmt.Errorf("failed to write example rule: %w", err)
	}

	return fmt.Sprintf("Created %s\n\nAdd .md files to this directory to define rules.\nExample created: %s", rulesDir, examplePath), nil
}

// addToGitignore adds an entry to .gitignore if it exists
func addToGitignore(cwd, entry string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return // .gitignore doesn't exist, skip
	}

	content := string(data)
	if strings.Contains(content, entry) {
		return // Already present
	}

	// Append entry
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	os.WriteFile(gitignorePath, []byte(content), 0644)
}

// handleMemoryCommand handles the /memory command
// Usage: /memory [list|show|edit] [global|project|local]
func handleMemoryCommand(ctx context.Context, m *model, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	// No arguments: show interactive selector (like Claude Code)
	if len(parts) == 0 {
		m.memorySelector.EnterMemorySelect(m.cwd, m.width, m.height)
		return "", nil
	}

	subCmd := strings.ToLower(parts[0])

	scope := "project"
	if len(parts) > 1 {
		scope = strings.ToLower(parts[1])
	}

	switch subCmd {
	case "list":
		return handleMemoryList(m)
	case "show":
		return handleMemoryShow(m)
	case "edit":
		return handleMemoryEdit(m, scope)
	default:
		return "Usage: /memory [list|show|edit] [global|project|local]", nil
	}
}

// memoryListState holds state for rendering memory list
type memoryListState struct {
	cwd        string
	totalFiles int
	totalSize  int64
}

const (
	memoryBoxWidth = 53
	memoryMaxPath  = 36
)

// handleMemoryList lists all loaded memory files with beautiful UI
func handleMemoryList(m *model) (string, error) {
	paths := system.GetAllMemoryPaths(m.cwd)
	state := &memoryListState{cwd: m.cwd}

	var sb strings.Builder

	sb.WriteString("╭─ Memory Files ─────────────────────────────────────╮\n")
	sb.WriteString(formatBoxLine(""))

	// Global section
	state.writeSection(&sb, "Global", paths.Global, paths.GlobalRules, paths.Global[0], false)

	// Project section
	state.writeSection(&sb, "Project", paths.Project, paths.ProjectRules, "/init", true)

	// Local section
	state.writeLocalSection(&sb, paths.Local)

	sb.WriteString("╰────────────────────────────────────────────────────╯\n")

	// Summary
	if state.totalFiles > 0 {
		fmt.Fprintf(&sb, "  Total: %d file(s) loaded (%s)\n", state.totalFiles, system.FormatFileSize(state.totalSize))
	} else {
		sb.WriteString("  No memory files loaded. Create with /init\n")
	}

	sb.WriteString("\n  Tip: Use @path/to/file.md in memory files to import other files.\n")

	return sb.String(), nil
}

// writeSection writes a memory section (Global or Project) to the builder
func (s *memoryListState) writeSection(sb *strings.Builder, label string, mainPaths []string, rulesDir, createHint string, isProject bool) {
	mainFound := system.FindMemoryFile(mainPaths)
	rulesFiles := system.ListRulesFiles(rulesDir)

	if mainFound != "" || len(rulesFiles) > 0 {
		sb.WriteString(formatBoxLine(fmt.Sprintf(" ● %s", label)))
		if mainFound != "" {
			s.writeFileLine(sb, mainFound, isProject)
		}
		for _, rf := range rulesFiles {
			s.writeFileLine(sb, rf, isProject)
		}
	} else {
		sb.WriteString(formatBoxLine(fmt.Sprintf(" ○ %s (not found)", label)))
		sb.WriteString(formatBoxLine(fmt.Sprintf("   Create: %s", createHint)))
	}
	sb.WriteString(formatBoxLine(""))
}

// writeLocalSection writes the local memory section
func (s *memoryListState) writeLocalSection(sb *strings.Builder, localPaths []string) {
	localFound := system.FindMemoryFile(localPaths)
	if localFound != "" {
		sb.WriteString(formatBoxLine(" ● Local (git-ignored)"))
		s.writeFileLine(sb, localFound, true)
	} else {
		sb.WriteString(formatBoxLine(" ○ Local (not found)"))
		sb.WriteString(formatBoxLine("   Create: /init local"))
	}
	sb.WriteString(formatBoxLine(""))
}

// writeFileLine writes a file entry line with size
func (s *memoryListState) writeFileLine(sb *strings.Builder, path string, isProject bool) {
	size := system.GetFileSize(path)
	s.totalFiles++
	s.totalSize += size

	displayPath := shortenPathForDisplay(path, s.cwd, isProject)
	displayPath = truncatePathKeepFilename(displayPath, memoryMaxPath)
	sizeStr := fmt.Sprintf("(%s)", system.FormatFileSize(size))
	sb.WriteString(formatBoxLine(fmt.Sprintf("   %s %s", padRight(displayPath, memoryMaxPath), sizeStr)))
}

// formatBoxLine formats a line with proper box alignment
func formatBoxLine(content string) string {
	visibleLen := utf8.RuneCountInString(content)
	padding := max(memoryBoxWidth-visibleLen-2, 0)
	return fmt.Sprintf("│ %s%s│\n", content, strings.Repeat(" ", padding))
}

// shortenPathForDisplay shortens a path for display
func shortenPathForDisplay(path, cwd string, isProject bool) string {
	if isProject {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			return rel
		}
	}
	return shortenPath(path)
}

// truncatePathKeepFilename truncates a path while keeping the filename visible
func truncatePathKeepFilename(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	base := filepath.Base(path)
	if len(base) >= maxLen-3 {
		return base[:maxLen-3] + "..."
	}

	remaining := maxLen - len(base) - 4
	if remaining > 0 {
		dir := filepath.Dir(path)
		if len(dir) > remaining {
			dir = dir[len(dir)-remaining:]
		}
		return "..." + dir + "/" + base
	}
	return base
}

// padRight pads a string to the right with spaces
func padRight(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
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
	paths := system.GetAllMemoryPaths(m.cwd)

	switch scope {
	case "global", "user":
		filePath, err := ensureMemoryFile(paths.Global, getGlobalTemplate())
		if err != nil {
			return "", err
		}
		m.editingMemoryFile = filePath
		return "", nil

	case "local":
		filePath, err := ensureMemoryFile(paths.Local, getLocalTemplate())
		if err != nil {
			return "", err
		}
		addToGitignore(m.cwd, "GEN.local.md")
		m.editingMemoryFile = filePath
		return "", nil

	default: // project
		filePath := system.FindMemoryFile(paths.Project)
		if filePath == "" {
			return "No project memory file found.\n\nCreate with: /init", nil
		}
		m.editingMemoryFile = filePath
		return "", nil
	}
}

// ensureMemoryFile finds or creates a memory file from the given paths.
// Returns the file path that was found or created.
func ensureMemoryFile(searchPaths []string, template string) (string, error) {
	filePath := system.FindMemoryFile(searchPaths)
	if filePath != "" {
		return filePath, nil
	}

	// Create the first path in the list
	filePath = searchPaths[0]
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(template), 0644); err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	return filePath, nil
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

// getGlobalTemplate returns the default global/user memory template
func getGlobalTemplate() string {
	return `# GEN.md

Global instructions for GenCode (applies to all projects).

## Coding Preferences

<!-- Your preferred coding style -->

## Security

<!-- Security practices to follow -->
`
}

// getLocalTemplate returns the default local memory template (git-ignored)
func getLocalTemplate() string {
	return `# GEN.local.md

Local instructions for this project (not committed to git).

Use this file for:
- Personal notes and reminders
- Environment-specific settings
- Credentials and secrets (keep these safe!)
- Work-in-progress ideas

## Notes

<!-- Your local notes here -->
`
}

// getRulesTemplate returns the default rules file template
func getRulesTemplate() string {
	return `# Example Rule

This file defines specific rules for GenCode to follow.

## Guidelines

- Add specific guidelines here
- Each rule file should focus on one topic
- Rules are loaded alphabetically by filename

## Example

<!-- Remove this example and add your actual rules -->
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

// handleMemorySelected handles when a memory file is selected from the selector
func (m model) handleMemorySelected(msg MemorySelectedMsg) (tea.Model, tea.Cmd) {
	filePath := msg.Path

	// Create file if it doesn't exist
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := createMemoryFile(filePath, msg.Level, m.cwd); err != nil {
			m.messages = append(m.messages, chatMessage{
				role:    roleNotice,
				content: fmt.Sprintf("Error: %v", err),
			})
			return m, tea.Batch(m.commitMessages()...)
		}
	}

	m.editingMemoryFile = filePath

	// Format display path based on level
	displayPath := formatMemoryDisplayPath(filePath, msg.Level, m.cwd)

	m.messages = append(m.messages, chatMessage{
		role:    roleNotice,
		content: fmt.Sprintf("Opening %s memory: %s", msg.Level, displayPath),
	})

	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, startExternalEditor(filePath))
	return m, tea.Batch(commitCmds...)
}

// createMemoryFile creates a new memory file with the appropriate template
func createMemoryFile(filePath, level, cwd string) error {
	template := getTemplateForLevel(level, cwd)
	if _, err := ensureMemoryFile([]string{filePath}, template); err != nil {
		return err
	}
	if level == "local" {
		addToGitignore(cwd, "GEN.local.md")
	}
	return nil
}

// getTemplateForLevel returns the appropriate template for the given memory level.
func getTemplateForLevel(level, cwd string) string {
	switch level {
	case "global":
		return getGlobalTemplate()
	case "project":
		return getProjectTemplate(cwd)
	case "local":
		return getLocalTemplate()
	default:
		return ""
	}
}

// formatMemoryDisplayPath formats a memory file path for display
func formatMemoryDisplayPath(filePath, level, cwd string) string {
	if level == "project" || level == "local" {
		if rel, err := filepath.Rel(cwd, filePath); err == nil {
			return rel
		}
	}
	return shortenPath(filePath)
}

// handleMCPCommand handles the /mcp command
// Usage: /mcp [add|remove|get|connect|disconnect|reconnect|list] [args...]
func handleMCPCommand(ctx context.Context, m *model, args string) (string, error) {
	if mcp.DefaultRegistry == nil {
		return "MCP is not initialized.\n\nAdd MCP servers with:\n  /mcp add <name> -- <command> [args...]", nil
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		// No arguments: enter interactive selector
		if err := m.mcpSelector.EnterMCPSelect(m.width, m.height); err != nil {
			return "", err
		}
		return "", nil
	}

	subCmd := strings.ToLower(parts[0])
	var serverName string
	if len(parts) > 1 {
		serverName = parts[1]
	}

	switch subCmd {
	case "add":
		return handleMCPAdd(ctx, m, parts[1:])
	case "remove":
		return handleMCPRemove(m, serverName)
	case "get":
		return handleMCPGet(m, serverName)
	case "connect":
		return handleMCPConnect(ctx, m, serverName)
	case "disconnect":
		return handleMCPDisconnect(m, serverName)
	case "reconnect":
		return handleMCPReconnect(ctx, m, serverName)
	case "list", "status":
		return handleMCPList(m)
	default:
		// Treat single argument as connect request
		return handleMCPConnect(ctx, m, subCmd)
	}
}

// handleMCPList lists all MCP servers and their status
func handleMCPList(m *model) (string, error) {
	servers := mcp.DefaultRegistry.List()

	if len(servers) == 0 {
		return "No MCP servers configured.\n\nAdd servers with:\n  /mcp add <name> -- <command> [args...]\n  /mcp add --transport http <name> <url>", nil
	}

	var sb strings.Builder
	sb.WriteString("MCP Servers:\n\n")

	for _, srv := range servers {
		icon, label := mcpStatusDisplay(srv.Status)
		scope := string(srv.Config.Scope)
		if scope == "" {
			scope = "local"
		}
		sb.WriteString(fmt.Sprintf("  %s %s [%s] (%s, %s)\n", icon, srv.Config.Name, srv.Config.GetType(), scope, label))

		if srv.Status == mcp.StatusConnected {
			if len(srv.Tools) > 0 {
				sb.WriteString(fmt.Sprintf("    Tools: %d\n", len(srv.Tools)))
			}
			if len(srv.Resources) > 0 {
				sb.WriteString(fmt.Sprintf("    Resources: %d\n", len(srv.Resources)))
			}
			if len(srv.Prompts) > 0 {
				sb.WriteString(fmt.Sprintf("    Prompts: %d\n", len(srv.Prompts)))
			}
		}

		if srv.Error != "" {
			sb.WriteString(fmt.Sprintf("    Error: %s\n", srv.Error))
		}
	}

	sb.WriteString("\nCommands:\n")
	sb.WriteString("  /mcp add <name> ...     Add a server\n")
	sb.WriteString("  /mcp remove <name>      Remove a server\n")
	sb.WriteString("  /mcp get <name>         Show server details\n")
	sb.WriteString("  /mcp connect <name>     Connect to server\n")
	sb.WriteString("  /mcp disconnect <name>  Disconnect from server\n")
	sb.WriteString("  /mcp reconnect <name>   Reconnect to server\n")

	return sb.String(), nil
}

// handleMCPConnect connects to an MCP server
func handleMCPConnect(ctx context.Context, m *model, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp connect <server-name>", nil
	}

	if _, ok := mcp.DefaultRegistry.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	if err := mcp.DefaultRegistry.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Failed to connect to %s: %v", name, err), nil
	}

	// Get connected server info
	if client, ok := mcp.DefaultRegistry.GetClient(name); ok {
		tools := client.GetCachedTools()
		return fmt.Sprintf("Connected to %s\nTools available: %d", name, len(tools)), nil
	}

	return fmt.Sprintf("Connected to %s", name), nil
}

// handleMCPDisconnect disconnects from an MCP server
func handleMCPDisconnect(m *model, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp disconnect <server-name>", nil
	}

	if err := mcp.DefaultRegistry.Disconnect(name); err != nil {
		return fmt.Sprintf("Failed to disconnect from %s: %v", name, err), nil
	}

	return fmt.Sprintf("Disconnected from %s", name), nil
}

// handleMCPAdd adds a new MCP server configuration
func handleMCPAdd(ctx context.Context, m *model, args []string) (string, error) {
	if len(args) == 0 {
		return mcpAddUsage(), nil
	}

	// Parse flags and positional arguments
	var (
		transport = "stdio"
		scope     = "local"
		envVars   []string
		headers   []string
		name      string
		positional []string
		dashIdx   = -1
	)

	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			dashIdx = i
			break
		}
		switch args[i] {
		case "--transport", "-t":
			if i+1 < len(args) {
				i++
				transport = args[i]
			}
		case "--scope", "-s":
			if i+1 < len(args) {
				i++
				scope = args[i]
			}
		case "--env", "-e":
			if i+1 < len(args) {
				i++
				envVars = append(envVars, args[i])
			}
		case "--header", "-H":
			if i+1 < len(args) {
				i++
				headers = append(headers, args[i])
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		return mcpAddUsage(), nil
	}
	name = positional[0]

	var config mcp.ServerConfig
	config.Type = mcp.TransportType(transport)

	switch config.Type {
	case mcp.TransportSTDIO:
		if dashIdx == -1 || dashIdx >= len(args)-1 {
			return "STDIO transport requires: /mcp add <name> -- <command> [args...]", nil
		}
		cmdArgs := args[dashIdx+1:]
		config.Command = cmdArgs[0]
		if len(cmdArgs) > 1 {
			config.Args = cmdArgs[1:]
		}

	case mcp.TransportHTTP, mcp.TransportSSE:
		if len(positional) < 2 {
			return fmt.Sprintf("%s transport requires a URL: /mcp add --transport %s <name> <url>", transport, transport), nil
		}
		config.URL = positional[1]
		config.Headers = parseMCPKeyValues(headers, ":")

	default:
		return fmt.Sprintf("Unsupported transport type: %s (use stdio, http, or sse)", transport), nil
	}

	config.Env = parseMCPKeyValues(envVars, "=")

	mcpScope := parseMCPScope(scope)
	if err := mcp.DefaultRegistry.AddServer(name, config, mcpScope); err != nil {
		return fmt.Sprintf("Failed to add server: %v", err), nil
	}

	// Auto-connect
	if err := mcp.DefaultRegistry.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Added '%s' to %s scope, but failed to connect: %v", name, scope, err), nil
	}

	toolCount := 0
	if client, ok := mcp.DefaultRegistry.GetClient(name); ok {
		toolCount = len(client.GetCachedTools())
	}

	return fmt.Sprintf("Added and connected to '%s' (%s, %s scope)\nTools available: %d", name, transport, scope, toolCount), nil
}

// handleMCPRemove removes an MCP server configuration
func handleMCPRemove(m *model, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp remove <server-name>", nil
	}

	if _, ok := mcp.DefaultRegistry.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	if err := mcp.DefaultRegistry.RemoveServer(name); err != nil {
		return fmt.Sprintf("Failed to remove %s: %v", name, err), nil
	}

	return fmt.Sprintf("Removed server '%s'", name), nil
}

// handleMCPGet shows detailed information about an MCP server
func handleMCPGet(m *model, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp get <server-name>", nil
	}

	config, ok := mcp.DefaultRegistry.GetConfig(name)
	if !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Server: %s\n", name))

	scope := string(config.Scope)
	if scope == "" {
		scope = "local"
	}
	sb.WriteString(fmt.Sprintf("Scope:  %s\n", scope))
	sb.WriteString(fmt.Sprintf("Type:   %s\n", config.GetType()))

	switch config.GetType() {
	case mcp.TransportSTDIO:
		cmd := config.Command
		if len(config.Args) > 0 {
			cmd += " " + strings.Join(config.Args, " ")
		}
		sb.WriteString(fmt.Sprintf("Command: %s\n", cmd))
	case mcp.TransportHTTP, mcp.TransportSSE:
		sb.WriteString(fmt.Sprintf("URL:    %s\n", config.URL))
	}

	if len(config.Env) > 0 {
		sb.WriteString("Env:\n")
		for k, v := range config.Env {
			// Mask values for security
			masked := v
			if len(masked) > 4 {
				masked = masked[:4] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s=%s\n", k, masked))
		}
	}

	if len(config.Headers) > 0 {
		sb.WriteString("Headers:\n")
		for k, v := range config.Headers {
			masked := v
			if len(masked) > 8 {
				masked = masked[:8] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, masked))
		}
	}

	// Connection status
	icon, label := mcpStatusDisplay(mcp.StatusDisconnected)
	toolCount := 0
	if client, ok := mcp.DefaultRegistry.GetClient(name); ok {
		srv := client.ToServer()
		icon, label = mcpStatusDisplay(srv.Status)
		toolCount = len(srv.Tools)

		if srv.Error != "" {
			sb.WriteString(fmt.Sprintf("Error:  %s\n", srv.Error))
		}
	}
	sb.WriteString(fmt.Sprintf("Status: %s %s\n", icon, label))
	if toolCount > 0 {
		sb.WriteString(fmt.Sprintf("Tools:  %d\n", toolCount))
	}

	return sb.String(), nil
}

// handleMCPReconnect disconnects and reconnects to an MCP server
func handleMCPReconnect(ctx context.Context, m *model, name string) (string, error) {
	if name == "" {
		return "Usage: /mcp reconnect <server-name>", nil
	}

	if _, ok := mcp.DefaultRegistry.GetConfig(name); !ok {
		return fmt.Sprintf("Server not found: %s\n\nUse /mcp list to see available servers.", name), nil
	}

	// Disconnect (ignore error if not connected)
	_ = mcp.DefaultRegistry.Disconnect(name)

	// Reconnect
	if err := mcp.DefaultRegistry.Connect(ctx, name); err != nil {
		return fmt.Sprintf("Failed to reconnect to %s: %v", name, err), nil
	}

	toolCount := 0
	if client, ok := mcp.DefaultRegistry.GetClient(name); ok {
		toolCount = len(client.GetCachedTools())
	}

	return fmt.Sprintf("Reconnected to %s\nTools available: %d", name, toolCount), nil
}

// parseMCPScope converts a string to mcp.Scope
func parseMCPScope(s string) mcp.Scope {
	switch strings.ToLower(s) {
	case "user", "global":
		return mcp.ScopeUser
	case "project":
		return mcp.ScopeProject
	default:
		return mcp.ScopeLocal
	}
}

// parseMCPKeyValues converts ["KEY=val", ...] to map[string]string
func parseMCPKeyValues(items []string, sep string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		if key, value, ok := strings.Cut(item, sep); ok {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}

// mcpAddUsage returns the help text for /mcp add
func mcpAddUsage() string {
	return `Usage: /mcp add [options] <name> [-- <command> [args...]] or <url>

Options:
  --transport <type>   Transport: stdio (default), http, sse
  --scope <scope>      Scope: local (default), project, user
  --env KEY=value      Environment variable (repeatable, STDIO only)
  --header Key:Value   HTTP header (repeatable, HTTP/SSE only)

Short flags: -t, -s, -e, -H

Examples:
  /mcp add myserver -- npx -y @modelcontextprotocol/server-filesystem .
  /mcp add --transport http pubmed https://pubmed.mcp.example.com/mcp
  /mcp add --transport http --scope project myapi https://api.example.com/mcp
  /mcp add --env API_KEY=xxx myserver -- npx -y some-mcp-server`
}

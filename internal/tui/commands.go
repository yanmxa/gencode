package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

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
		"tool": {
			Name:        "tool",
			Description: "Manage available tools (enable/disable)",
			Handler:     handleToolCommand,
		},
		"plan": {
			Name:        "plan",
			Description: "Enter plan mode to explore and plan before execution",
			Handler:     handlePlanCommand,
		},
		"skill": {
			Name:        "skill",
			Description: "Manage skills (enable/disable/activate)",
			Handler:     handleSkillCommand,
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
	// Clear all messages
	m.messages = []chatMessage{}
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

// handleSkillCommand handles the /skill command
func handleSkillCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.skillSelector.EnterSkillSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
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

package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

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
		"read": {
			Name:        "read",
			Description: "Read file contents",
			Handler:     handleReadCommand,
		},
		"glob": {
			Name:        "glob",
			Description: "Find files matching a pattern",
			Handler:     handleGlobCommand,
		},
		"grep": {
			Name:        "grep",
			Description: "Search for patterns in files",
			Handler:     handleGrepCommand,
		},
		"fetch": {
			Name:        "fetch",
			Description: "Fetch content from a URL",
			Handler:     handleFetchCommand,
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

	registry := getCommandRegistry()
	command, ok := registry[cmd]
	if !ok {
		return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd), true
	}

	result, err := command.Handler(ctx, m, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), true
	}
	return result, true
}

// GetMatchingCommands returns commands matching the prefix for fuzzy search
func GetMatchingCommands(prefix string) []Command {
	prefix = strings.ToLower(strings.TrimPrefix(prefix, "/"))
	matches := make([]Command, 0)

	registry := getCommandRegistry()
	for name, cmd := range registry {
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, cmd)
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

// handleReadCommand handles the /read command
func handleReadCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /read <file_path>", nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"file_path": args}

	result := tool.Execute(ctx, "read", params, cwd)
	return ui.RenderToolResult(result, m.width), nil
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

// handleGrepCommand handles the /grep command
func handleGrepCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /grep <pattern> [path]", nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"pattern": args}

	// Check if a path is specified
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "grep", params, cwd)
	return ui.RenderToolResult(result, m.width), nil
}

// handleFetchCommand handles the /fetch command
func handleFetchCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /fetch <url>", nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"url": args}

	result := tool.Execute(ctx, "webfetch", params, cwd)
	return ui.RenderToolResult(result, m.width), nil
}

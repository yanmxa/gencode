// Slash command registry, dispatch, and basic command handlers
// (/help, /clear, /provider, /model, /tools, /skills, /agents, /plan, /glob).
package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

type CommandHandler func(ctx context.Context, m *model, args string) (string, error)

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
		"plugin": {
			Name:        "plugin",
			Description: "Manage plugins (list/enable/disable/info)",
			Handler:     handlePluginCommand,
		},
	}
}

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

func ExecuteCommand(ctx context.Context, m *model, input string) (string, bool) {
	cmd, args, isCmd := ParseCommand(input)
	if !isCmd {
		return "", false
	}

	registry := getCommandRegistry()
	command, ok := registry[cmd]
	if ok {
		result, err := command.Handler(ctx, m, args)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		return result, true
	}

	if sk, ok := IsSkillCommand(cmd); ok {
		return executeSkillCommand(m, sk, args), true
	}

	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd), true
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) string {
	if skill.DefaultRegistry != nil {
		m.pendingSkillInstructions = skill.DefaultRegistry.GetSkillInvocationPrompt(sk.FullName())
	}

	if args != "" {
		m.pendingSkillArgs = args
	} else {
		m.pendingSkillArgs = fmt.Sprintf("/%s", sk.FullName())
	}

	return ""
}

func GetMatchingCommands(query string) []Command {
	query = strings.ToLower(strings.TrimPrefix(query, "/"))
	matches := make([]Command, 0)

	registry := getCommandRegistry()
	for name, cmd := range registry {
		if FuzzyMatch(name, query) {
			matches = append(matches, cmd)
		}
	}

	skillCmds := GetSkillCommands()
	for _, cmd := range skillCmds {
		if FuzzyMatch(strings.ToLower(cmd.Name), query) {
			if _, exists := registry[cmd.Name]; !exists {
				matches = append(matches, cmd)
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	return matches
}

func IsSkillCommand(cmd string) (*skill.Skill, bool) {
	if skill.DefaultRegistry == nil {
		return nil, false
	}

	s, ok := skill.DefaultRegistry.Get(cmd)
	if !ok {
		return nil, false
	}

	if !s.IsEnabled() {
		return nil, false
	}

	return s, true
}

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
		cmds = append(cmds, Command{
			Name:        s.FullName(),
			Description: s.Description + hint,
		})
	}
	return cmds
}

func handleProviderCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.selector.EnterProviderSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

func handleModelCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.selector.EnterModelSelect(ctx, m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

func handleHelpCommand(ctx context.Context, m *model, args string) (string, error) {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	registry := getCommandRegistry()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmd := registry[name]
		fmt.Fprintf(&sb, "  /%s - %s\n", cmd.Name, cmd.Description)
	}

	return sb.String(), nil
}

func handleClearCommand(ctx context.Context, m *model, args string) (string, error) {
	m.messages = []chatMessage{}
	m.committedCount = 0
	m.lastInputTokens = 0
	m.lastOutputTokens = 0
	m.pendingClearScreen = true
	tool.DefaultTodoStore.Reset()
	return "", nil
}

func handleGlobCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"pattern": args}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "glob", params, cwd)
	return ui.RenderToolResult(result, m.width), nil
}

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

func handlePlanCommand(ctx context.Context, m *model, args string) (string, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil
	}

	m.operationMode = modePlan
	m.planMode = true
	m.planTask = args

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

func handleSkillCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.skillSelector.EnterSkillSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

func handleAgentCommand(ctx context.Context, m *model, args string) (string, error) {
	if err := m.agentSelector.EnterAgentSelect(m.width, m.height); err != nil {
		return "", err
	}
	return "", nil
}

func formatTokenCount(count int) string {
	if count >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	if count >= 1000 {
		return fmt.Sprintf("%dK", count/1000)
	}
	return fmt.Sprintf("%d", count)
}

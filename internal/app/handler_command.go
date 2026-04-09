// handler_command.go contains the core command dispatch logic: the CommandHandler
// type, the builtin handler registry, ExecuteCommand, and the /help command.
//
// Individual command handlers are split into focused files:
//   - handler_command_session.go  — /clear, /fork, /resume
//   - handler_command_config.go   — /provider, /model, /init, /memory, /mcp, /plugin, /reload-plugins
//   - handler_command_tool.go     — /tools, /glob, /skills, /agents
//   - handler_command_mode.go     — /plan, /think
//   - handler_command_loop.go     — /loop (scheduling, parsing, admin sub-commands)
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/command"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/skill"
)

type CommandHandler func(ctx context.Context, m *model, args string) (string, tea.Cmd, error)

var builtinCommandHandlers = map[string]CommandHandler{
	"provider":       handleProviderCommand,
	"model":          handleModelCommand,
	"clear":          handleClearCommand,
	"fork":           handleForkCommand,
	"resume":         handleResumeCommand,
	"help":           handleHelpCommand,
	"glob":           handleGlobCommand,
	"tools":          handleToolCommand,
	"plan":           handlePlanCommand,
	"skills":         handleSkillCommand,
	"agents":         handleAgentCommand,
	"tokenlimit":     handleTokenLimitCommand,
	"compact":        handleCompactCommand,
	"init":           handleInitCommand,
	"memory":         handleMemoryCommand,
	"mcp":            handleMCPCommand,
	"plugin":         handlePluginCommand,
	"reload-plugins": handleReloadPluginsCommand,
	"think":          handleThinkCommand,
	"loop":           handleLoopCommand,
}

// handlerRegistry maps command names to their handler functions.
// The set of names must match command.BuiltinNames().
func handlerRegistry() map[string]CommandHandler {
	return builtinCommandHandlers
}

func ExecuteCommand(ctx context.Context, m *model, input string) (string, tea.Cmd, bool) {
	cmd, args, isCmd := command.ParseCommand(input)
	if !isCmd {
		return "", nil, false
	}

	if result, followUp, handled := executeExitCommand(m, cmd); handled {
		return result, followUp, true
	}

	if result, followUp, handled := executeBuiltinCommand(ctx, m, cmd, args); handled {
		return result, followUp, true
	}

	if sk, ok := command.IsSkillCommand(cmd); ok {
		return executeSkillSlashCommand(m, sk, args), m.handleSkillInvocation(), true
	}

	if pc, ok := command.IsCustomCommand(cmd); ok {
		return executeCustomCommand(m, pc, args), m.handleSkillInvocation(), true
	}

	return unknownCommandResult(cmd), nil, true
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) {
	if skill.DefaultRegistry != nil {
		m.skill.PendingInstructions = skill.DefaultRegistry.GetSkillInvocationPrompt(sk.FullName())
	}

	plugin.SetActivePluginRoot(plugin.FindPluginRootForPath(sk.SkillDir))

	if args != "" {
		m.skill.PendingArgs = fmt.Sprintf("/%s %s", sk.FullName(), args)
	} else {
		m.skill.PendingArgs = fmt.Sprintf("/%s", sk.FullName())
	}
}

func executeExitCommand(m *model, cmd string) (string, tea.Cmd, bool) {
	if cmd != "exit" {
		return "", nil, false
	}
	if m.conv.Stream.Cancel != nil {
		m.conv.Stream.Cancel()
	}
	m.fireSessionEnd("prompt_input_exit")
	return "", tea.Quit, true
}

func executeBuiltinCommand(ctx context.Context, m *model, cmd, args string) (string, tea.Cmd, bool) {
	handler, ok := handlerRegistry()[cmd]
	if !ok {
		return "", nil, false
	}

	result, followUp, err := handler(ctx, m, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil, true
	}
	return result, followUp, true
}

func executeSkillSlashCommand(m *model, sk *skill.Skill, args string) string {
	executeSkillCommand(m, sk, args)
	return ""
}

func executeCustomCommand(m *model, pc *command.CustomCommand, args string) string {
	instructions := pc.GetInstructions()
	if instructions != "" {
		m.skill.PendingInstructions = fmt.Sprintf("<custom-command name=%q>\n%s\n</custom-command>", pc.FullName(), instructions)
	}

	plugin.SetActivePluginRoot(plugin.FindPluginRootForPath(pc.FilePath))

	if args != "" {
		m.skill.PendingArgs = fmt.Sprintf("/%s %s", pc.FullName(), args)
	} else {
		m.skill.PendingArgs = fmt.Sprintf("/%s", pc.FullName())
	}
	return ""
}

func unknownCommandResult(cmd string) string {
	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd)
}

func handleHelpCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	builtins := command.BuiltinNames()

	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		info := builtins[name]
		fmt.Fprintf(&sb, "  /%s - %s\n", info.Name, info.Description)
	}

	pluginCmds := command.GetCustomCommands()
	if len(pluginCmds) > 0 {
		sb.WriteString("\nCustom Commands:\n\n")
		for _, cmd := range pluginCmds {
			desc := cmd.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&sb, "  /%s - %s\n", cmd.Name, desc)
		}
	}

	return sb.String(), nil, nil
}

// handler_command.go contains the core command dispatch logic: the commandHandler
// type, the builtin handler registry, executeCommand, and the /help command.
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

	"github.com/yanmxa/gencode/internal/ext/command"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/plugin"
)

type commandHandler func(ctx context.Context, m *model, args string) (string, tea.Cmd, error)

var builtinCommandHandlers = map[string]commandHandler{
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
	"search":         handleSearchCommand,
}

// handlerRegistry maps command names to their handler functions.
// The set of names must match command.BuiltinNames().
func handlerRegistry() map[string]commandHandler {
	return builtinCommandHandlers
}

func executeCommand(ctx context.Context, m *model, input string) (string, tea.Cmd, bool) {
	return m.commands().execute(ctx, input)
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
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
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

func lookupSkillCommand(cmd string) (*skill.Skill, bool) {
	if skill.DefaultRegistry == nil {
		return nil, false
	}

	sk, ok := skill.DefaultRegistry.Get(cmd)
	if !ok || !sk.IsEnabled() {
		return nil, false
	}
	return sk, true
}

func skillCommandInfos() []command.Info {
	if skill.DefaultRegistry == nil {
		return nil
	}

	enabled := skill.DefaultRegistry.GetEnabled()
	infos := make([]command.Info, 0, len(enabled))
	for _, sk := range enabled {
		description := sk.Description
		if sk.ArgumentHint != "" {
			description += " " + sk.ArgumentHint
		}
		infos = append(infos, command.Info{
			Name:        sk.FullName(),
			Description: description,
		})
	}
	return infos
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

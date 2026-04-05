package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/command"
	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type CommandHandler func(ctx context.Context, m *model, args string) (string, tea.Cmd, error)

// handlerRegistry maps command names to their handler functions.
// The set of names must match command.BuiltinNames().
func handlerRegistry() map[string]CommandHandler {
	return map[string]CommandHandler{
		"provider":   handleProviderCommand,
		"model":      handleModelCommand,
		"clear":      handleClearCommand,
		"fork":       handleForkCommand,
		"resume":     handleResumeCommand,
		"help":       handleHelpCommand,
		"glob":       handleGlobCommand,
		"tools":      handleToolCommand,
		"plan":       handlePlanCommand,
		"skills":     handleSkillCommand,
		"agents":     handleAgentCommand,
		"tokenlimit": handleTokenLimitCommand,
		"compact":    handleCompactCommand,
		"init": func(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
			result, err := appmemory.HandleInitCommand(m.cwd, args)
			return result, nil, err
		},
		"memory": func(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
			result, editPath, err := appmemory.HandleMemoryCommand(&m.memory.Selector, m.cwd, m.width, m.height, args)
			if err != nil {
				return "", nil, err
			}
			if editPath != "" {
				m.memory.EditingFile = editPath
				return result, startExternalEditor(editPath), nil
			}
			return result, nil, nil
		},
		"mcp": func(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
			result, editInfo, err := appmcp.HandleCommand(ctx, &m.mcp.Selector, m.width, m.height, args)
			if err != nil {
				return "", nil, err
			}
			if editInfo != nil {
				m.mcp.EditingFile = editInfo.TempFile
				m.mcp.EditingServer = editInfo.ServerName
				m.mcp.EditingScope = editInfo.Scope
				return result, startMCPEditor(editInfo.TempFile), nil
			}
			if m.mcp.Selector.IsActive() {
				return result, m.mcp.Selector.AutoReconnect(), nil
			}
			return result, nil, nil
		},
		"plugin": func(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
			result, err := appplugin.HandleCommand(ctx, &m.plugin.Selector, m.cwd, m.width, m.height, args)
			return result, nil, err
		},
		"think": handleThinkCommand,
	}
}

func ExecuteCommand(ctx context.Context, m *model, input string) (string, tea.Cmd, bool) {
	cmd, args, isCmd := command.ParseCommand(input)
	if !isCmd {
		return "", nil, false
	}

	// Handle /exit like CC does
	if cmd == "exit" {
		if m.conv.Stream.Cancel != nil {
			m.conv.Stream.Cancel()
		}
		m.fireSessionEnd("prompt_input_exit")
		return "", tea.Quit, true
	}

	handlers := handlerRegistry()
	if handler, ok := handlers[cmd]; ok {
		result, followUp, err := handler(ctx, m, args)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), nil, true
		}
		return result, followUp, true
	}

	if sk, ok := command.IsSkillCommand(cmd); ok {
		executeSkillCommand(m, sk, args)
		followUp := m.handleSkillInvocation()
		return "", followUp, true
	}

	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd), nil, true
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) {
	if skill.DefaultRegistry != nil {
		m.skill.PendingInstructions = skill.DefaultRegistry.GetSkillInvocationPrompt(sk.FullName())
	}

	if args != "" {
		m.skill.PendingArgs = args
	} else {
		m.skill.PendingArgs = fmt.Sprintf("/%s", sk.FullName())
	}
}

func handleProviderCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.provider.Selector.EnterProviderSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleModelCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.provider.Selector.EnterModelSelect(ctx, m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
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

	return sb.String(), nil, nil
}

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	m.conv.Clear()
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0
	tool.DefaultTodoStore.Reset()
	tool.ResetFetched()
	cron.DefaultStore.Reset()
	m.cronQueue = nil
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		_, _ = tty.WriteString("\033[2J\033[3J\033[H")
		_ = tty.Close()
	}
	if os.Getenv("TMUX") != "" {
		_ = exec.Command("tmux", "clear-history").Run()
	}
	return "", tea.ClearScreen, nil
}

func handleForkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if len(m.conv.Messages) == 0 {
		return "Nothing to fork — no messages in current session.", nil, nil
	}

	// Save current session first so all messages are persisted.
	if err := m.saveSession(); err != nil {
		return "", nil, fmt.Errorf("failed to save session before fork: %w", err)
	}

	if m.session.CurrentID == "" {
		return "No active session to fork.", nil, nil
	}

	forked, err := m.session.Store.Fork(m.session.CurrentID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fork session: %w", err)
	}

	// Switch to the forked session.
	m.session.CurrentID = forked.Metadata.ID
	m.session.Summary = ""
	tool.DefaultTodoStore.SetStorageDir("")
	m.initTaskStorage()

	m.reconfigureAgentTool()

	originalID := forked.Metadata.ParentSessionID
	return fmt.Sprintf("Forked conversation. You are now in the fork.\nTo resume the original: gen -r %s", originalID), nil, nil
}

func handleResumeCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.ensureSessionStore(); err != nil {
		return "", nil, fmt.Errorf("failed to initialize session store: %w", err)
	}
	if err := m.session.Selector.EnterSelect(m.width, m.height, m.session.Store, m.cwd); err != nil {
		return "", nil, fmt.Errorf("failed to open session selector: %w", err)
	}
	return "", nil, nil
}

func handleGlobCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil, nil
	}

	cwd, _ := os.Getwd()
	params := map[string]any{"pattern": args}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "glob", params, cwd)
	return ui.RenderToolResult(result, m.width), nil, nil
}

func handleToolCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	var mcpTools func() []provider.Tool
	if m.mcp.Registry != nil {
		mcpTools = m.mcp.Registry.GetToolSchemas
	}
	if err := m.tool.Selector.EnterSelect(m.width, m.height, m.mode.DisabledTools, mcpTools); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handlePlanCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil, nil
	}

	m.mode.Operation = appmode.Plan
	m.mode.Enabled = true
	m.mode.Task = args

	m.mode.SessionPermissions.AllowAllEdits = false
	m.mode.SessionPermissions.AllowAllWrites = false
	m.mode.SessionPermissions.AllowAllBash = false
	m.mode.SessionPermissions.AllowAllSkills = false

	store, err := plan.NewStore()
	if err != nil {
		return "", nil, fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.mode.Store = store

	return fmt.Sprintf("Entering plan mode for: %s\n\nI will explore the codebase and create an implementation plan. Only read-only tools are available until the plan is approved.", args), nil, nil
}

func handleSkillCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.skill.Selector.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleAgentCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.agent.Selector.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleThinkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(strings.ToLower(args))

	switch args {
	case "off", "0":
		m.provider.ThinkingLevel = provider.ThinkingOff
	case "", "toggle":
		// Cycle to next level
		m.provider.ThinkingLevel = m.provider.ThinkingLevel.Next()
	case "think", "normal", "1":
		m.provider.ThinkingLevel = provider.ThinkingNormal
	case "think+", "high", "2":
		m.provider.ThinkingLevel = provider.ThinkingHigh
	case "ultra", "ultrathink", "max", "3":
		m.provider.ThinkingLevel = provider.ThinkingUltra
	default:
		return "Usage: /think [off|think|think+|ultra]\n\nLevels:\n  off        — No extended thinking\n  think      — Moderate thinking budget\n  think+     — Extended thinking budget\n  ultra      — Maximum thinking budget\n\nWithout arguments, cycles to the next level.", nil, nil
	}

	m.provider.StatusMessage = fmt.Sprintf("thinking: %s", m.provider.ThinkingLevel.String())
	return "", appprovider.StatusTimer(3 * time.Second), nil
}

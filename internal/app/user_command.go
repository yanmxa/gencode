// Command dispatch: registry, routing, and all builtin command handlers.
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

	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/app/output/render"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
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

// commandController owns slash-command execution and transcript insertion rules.
type commandController struct {
	model *model
}

func (m *model) commands() commandController {
	return commandController{model: m}
}

func (c commandController) execute(ctx context.Context, input string) (string, tea.Cmd, bool) {
	cmd, args, isCmd := command.ParseCommand(input)
	if !isCmd {
		return "", nil, false
	}

	if result, followUp, handled := executeExitCommand(c.model, cmd); handled {
		return result, followUp, true
	}

	if result, followUp, handled := executeBuiltinCommand(ctx, c.model, cmd, args); handled {
		return result, followUp, true
	}

	if sk, ok := lookupSkillCommand(cmd); ok {
		return executeSkillSlashCommand(c.model, sk, args), c.model.handleSkillInvocation(), true
	}

	if pc, ok := command.IsCustomCommand(cmd); ok {
		return executeCustomCommand(c.model, pc, args), c.model.handleSkillInvocation(), true
	}

	return unknownCommandResult(cmd), nil, true
}

func (c commandController) handleSubmit(input string) (tea.Cmd, bool) {
	preserve := shouldPreserveCommandInConversation(input)
	preAppended := false
	if preserve && shouldPreserveBeforeCommandExecution(input) {
		c.model.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: input})
		preAppended = true
	}

	insertAt := len(c.model.conv.Messages)
	result, cmd, isCmd := c.execute(context.Background(), input)
	if !isCmd {
		if preAppended && len(c.model.conv.Messages) > 0 {
			c.model.conv.Messages = c.model.conv.Messages[:len(c.model.conv.Messages)-1]
		}
		return nil, false
	}

	c.model.resetInputField()

	if preserve && !preAppended {
		c.insertConversationMessage(insertAt, core.ChatMessage{Role: core.RoleUser, Content: input})
	}
	if result != "" {
		c.model.conv.AddNotice(result)
	}

	cmds := c.model.commitMessages()
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...), true
}

func (c commandController) insertConversationMessage(idx int, msg core.ChatMessage) {
	if idx < 0 || idx >= len(c.model.conv.Messages) {
		c.model.conv.Append(msg)
		return
	}

	c.model.conv.Messages = append(c.model.conv.Messages, core.ChatMessage{})
	copy(c.model.conv.Messages[idx+1:], c.model.conv.Messages[idx:])
	c.model.conv.Messages[idx] = msg
	if idx < c.model.conv.CommittedCount {
		c.model.conv.CommittedCount++
	}
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

// --- Session commands: /clear, /fork, /resume ---

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	if m.tool.Cancel != nil {
		m.tool.Cancel()
	}
	m.tool.Reset()

	m.conv.Clear()
	m.inputTokens = 0
	m.outputTokens = 0
	tracker.DefaultStore.Reset()
	tool.ResetFetched()
	m.systemInput.CronQueue = nil
	cmds := []tea.Cmd{tea.ClearScreen}
	if os.Getenv("TMUX") != "" {
		cmds = append(cmds, func() tea.Msg {
			_ = exec.Command("tmux", "clear-history").Run()
			return nil
		})
	}
	return "", tea.Batch(cmds...), nil
}

func handleForkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if len(m.conv.Messages) == 0 {
		return "Nothing to fork — no messages in current session.", nil, nil
	}

	if err := m.saveSession(); err != nil {
		return "", nil, fmt.Errorf("failed to save session before fork: %w", err)
	}

	if m.sessionID == "" {
		return "No active session to fork.", nil, nil
	}

	forked, err := m.sessionStore.Fork(m.sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fork session: %w", err)
	}

	m.sessionID = forked.Metadata.ID
	m.sessionSummary = ""
	tracker.DefaultStore.SetStorageDir("")
	m.initTaskStorage()

	m.reconfigureAgentTool()

	originalID := forked.Metadata.ParentSessionID
	return fmt.Sprintf("Forked conversation. You are now in the fork.\nTo resume the original: gen -r %s", originalID), nil, nil
}

func handleResumeCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.ensureSessionStore(); err != nil {
		return "", nil, fmt.Errorf("failed to initialize session store: %w", err)
	}
	if err := m.session.Selector.EnterSelect(m.width, m.height, m.sessionStore, m.cwd); err != nil {
		return "", nil, fmt.Errorf("failed to open session selector: %w", err)
	}
	return "", nil, nil
}

// --- Config commands: /model, /init, /memory, /mcp, /plugin, /reload-plugins, /search ---

func handleSearchCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.search.Enter(m.providerStore, m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleModelCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	cmd, err := m.provider.Selector.Enter(ctx, m.width, m.height)
	if err != nil {
		return "", nil, err
	}
	return "", cmd, nil
}

func handleInitCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, err := appmemory.HandleInitCommand(m.cwd, args)
	return result, nil, err
}

func handleMemoryCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, editPath, err := appmemory.HandleMemoryCommand(&m.memory.Selector, m.cwd, m.width, m.height, args)
	if err != nil {
		return "", nil, err
	}
	if editPath != "" {
		m.memory.EditingFile = editPath
		return result, startExternalEditor(editPath), nil
	}
	return result, nil, nil
}

func handleMCPCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, editInfo, err := mcpui.HandleCommand(ctx, &m.mcp.Selector, m.width, m.height, args)
	if err != nil {
		return "", nil, err
	}
	if editInfo != nil {
		m.mcp.EditingFile = editInfo.TempFile
		m.mcp.EditingServer = editInfo.ServerName
		m.mcp.EditingScope = editInfo.Scope
		return result, mcpui.StartMCPEditor(editInfo.TempFile), nil
	}
	if m.mcp.Selector.IsActive() {
		return result, m.mcp.Selector.AutoReconnect(), nil
	}
	return result, nil, nil
}

func handlePluginCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, err := pluginui.HandleCommand(ctx, &m.plugin, m.cwd, m.width, m.height, args)
	return result, nil, err
}

func handleReloadPluginsCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if strings.TrimSpace(args) != "" {
		return "Usage: /reload-plugins", nil, nil
	}

	if err := plugin.DefaultRegistry.Load(ctx, m.cwd); err != nil {
		return "", nil, fmt.Errorf("failed to reload plugin registry: %w", err)
	}
	_ = plugin.DefaultRegistry.LoadClaudePlugins(ctx)

	if err := m.reloadPluginBackedState(); err != nil {
		return "", nil, err
	}

	return "Reloaded plugins and refreshed plugin-backed skills, agents, MCP servers, and hooks.", nil, nil
}

// --- Tool commands: /tools, /glob, /skills, /agents ---

func handleGlobCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil, nil
	}

	params := map[string]any{"pattern": args}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}

	result := tool.Execute(ctx, "glob", params, m.cwd)
	return render.RenderToolResult(result, m.width), nil, nil
}

func handleToolCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	var mcpTools func() []core.ToolSchema
	if mcp.DefaultRegistry != nil {
		mcpTools = mcp.DefaultRegistry.GetToolSchemas
	}
	if err := m.tool.Selector.EnterSelect(m.width, m.height, m.disabledTools, mcpTools); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleSkillCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.skill.Selector.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func handleAgentCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.agent.EnterSelect(m.width, m.height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

// --- Mode commands: /plan, /think ---

func handlePlanCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil, nil
	}

	m.operationMode = config.ModePlan
	m.planEnabled = true
	m.planTask = args

	m.sessionPermissions.AllowAllEdits = false
	m.sessionPermissions.AllowAllWrites = false
	m.sessionPermissions.AllowAllBash = false
	m.sessionPermissions.AllowAllSkills = false

	store, err := plan.NewStore()
	if err != nil {
		return "", nil, fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.planStore = store

	return fmt.Sprintf("Entering plan mode for: %s\n\nI will explore the codebase and create an implementation plan. Only read-only tools are available until the plan is approved.", args), nil, nil
}

func handleThinkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(strings.ToLower(args))

	switch args {
	case "off", "0":
		m.thinkingLevel = llm.ThinkingOff
	case "", "toggle":
		m.thinkingLevel = m.thinkingLevel.Next()
	case "think", "normal", "1":
		m.thinkingLevel = llm.ThinkingNormal
	case "think+", "high", "2":
		m.thinkingLevel = llm.ThinkingHigh
	case "ultra", "ultrathink", "max", "3":
		m.thinkingLevel = llm.ThinkingUltra
	default:
		return "Usage: /think [off|think|think+|ultra]\n\nLevels:\n  off        — No extended thinking\n  think      — Moderate thinking budget\n  think+     — Extended thinking budget\n  ultra      — Maximum thinking budget\n\nWithout arguments, cycles to the next level.", nil, nil
	}

	m.provider.StatusMessage = fmt.Sprintf("thinking: %s", m.thinkingLevel.String())
	return "", providerui.StatusTimer(3 * time.Second), nil
}

// --- Loop commands: /loop ---

func handleLoopCommand(_ context.Context, m *model, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return loopUsage(), nil, nil
	}
	if result, handled, err := handleLoopAdminCommand(args); handled {
		return result, nil, err
	}
	if strings.HasPrefix(strings.ToLower(args), "once ") {
		parsed, err := cron.ParseLoopOnceCommand(strings.TrimSpace(args[5:]), time.Now())
		if err != nil {
			return loopUsage(), nil, nil
		}

		job, err := cron.DefaultStore.Create(parsed.Cron, parsed.Prompt, false, false)
		if err != nil {
			return "", nil, err
		}

		if m.conv.Messages == nil {
			m.conv = appconv.New()
		}
		m.conv.AddNotice(fmt.Sprintf(
			"Scheduled one-shot task %s (%s, cron `%s`).%s It will fire once and auto-delete.",
			job.ID,
			parsed.Human,
			parsed.Cron,
			parsed.Note,
		))
		return "", nil, nil
	}

	parsed, err := cron.ParseLoopCommand(args, time.Now())
	if err != nil {
		return loopUsage(), nil, nil
	}

	job, err := cron.DefaultStore.Create(parsed.Cron, parsed.Prompt, true, false)
	if err != nil {
		return "", nil, err
	}

	if m.conv.Messages == nil {
		m.conv = appconv.New()
	}

	m.conv.AddNotice(fmt.Sprintf(
		"Scheduled recurring task %s (%s, cron `%s`).%s Auto-expires after 7 days. Executing now.",
		job.ID,
		parsed.Human,
		parsed.Cron,
		parsed.Note,
	))
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: parsed.Prompt,
	})

	return "", m.startProviderTurn(parsed.Prompt), nil
}

func handleLoopAdminCommand(args string) (string, bool, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", false, nil
	}

	switch strings.ToLower(fields[0]) {
	case "list", "ls":
		return renderLoopJobList(), true, nil
	case "delete", "del", "rm", "remove", "cancel":
		if len(fields) < 2 {
			return "Usage: /loop delete <job-id>", true, nil
		}
		if strings.EqualFold(fields[1], "all") {
			jobs := cron.DefaultStore.List()
			for _, job := range jobs {
				if err := cron.DefaultStore.Delete(job.ID); err != nil {
					return "", true, err
				}
			}
			return fmt.Sprintf("Cancelled %d scheduled task(s).", len(jobs)), true, nil
		}
		id := strings.TrimSpace(fields[1])
		if id == "" {
			return "Usage: /loop delete <job-id>", true, nil
		}
		if err := cron.DefaultStore.Delete(id); err != nil {
			return "", true, err
		}
		return fmt.Sprintf("Cancelled scheduled task %s.", id), true, nil
	default:
		return "", false, nil
	}
}

func renderLoopJobList() string {
	jobs := cron.DefaultStore.List()
	if len(jobs) == 0 {
		return "No scheduled loop tasks."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d scheduled loop task(s):\n\n", len(jobs)))
	for _, job := range jobs {
		mode := "recurring"
		if !job.Recurring {
			mode = "one-shot"
		}
		if job.Durable {
			mode += ", durable"
		}
		sb.WriteString(fmt.Sprintf("%s  %s (%s)\n", job.ID, cron.Describe(job.Cron), mode))
		sb.WriteString(fmt.Sprintf("  Cron: %s\n", job.Cron))
		sb.WriteString(fmt.Sprintf("  Prompt: %s\n", job.Prompt))
		sb.WriteString(fmt.Sprintf("  Next: %s\n\n", job.NextFire.Format("2006-01-02 15:04")))
	}

	return sb.String()
}

func loopUsage() string {
	return "Usage: /loop [interval] <prompt>\n       /loop once <interval> <prompt>\n       /loop once <prompt> in <interval>\n       /loop list\n       /loop delete <job-id>\n       /loop delete all\nExamples: /loop 5m check the deploy, /loop check the deploy every 20m, /loop once 20m check the deploy"
}

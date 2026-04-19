package input

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

type commandHandler func(*CommandController, context.Context, string) (string, tea.Cmd, error)

type CommandDeps struct {
	Input        *Model
	Conversation *conv.ConversationModel
	Tool         *conv.ToolExecState
	Width        int
	Height       int
	Cwd          string

	// Read-only state
	DisabledTools map[string]bool
	ProviderStore *llm.Store
	LLMProvider   llm.Provider
	InputTokens   int
	CurrentModel  *llm.CurrentModelInfo

	// Domain services
	Skill   skill.Service
	Plugin  plugin.Service
	MCP     mcp.Service
	Tracker tracker.Service
	Cron    cron.Service
	ToolSvc tool.Service
	Command command.Service

	// State getters (values that may change during command execution)
	GetSessionID     func() string
	GetSessionStore  func() *session.Store
	GetThinkingLevel func() llm.ThinkingLevel

	// Mutation callbacks
	ResetTokens        func()
	SetThinkingLevel   func(llm.ThinkingLevel)
	EnterPlanMode      func(task string) error
	EnsureSessionStore func(cwd string) error
	ForkSession        func() (originalSessionID string, err error)
	ResetFetched       func()

	// Existing callbacks
	CommitMessages          func() []tea.Cmd
	StartProviderTurn       func(content string) tea.Cmd
	HandleSkillInvocation   func() tea.Cmd
	StartExternalEditor     func(path string) tea.Cmd
	ReloadPluginBackedState func() error
	PersistSession          func() error
	InitTaskStorage         func()
	ReconfigureAgentTool    func()
	StopAgentSession        func()
	FireSessionEnd          func(reason string)
	BuildCompactRequest     func(focus, trigger string) conv.CompactRequest
	SpinnerTickCmd          func() tea.Cmd
	ResetCronQueue          func()
}

type CommandController struct {
	deps CommandDeps
}

func NewCommandController(deps CommandDeps) CommandController {
	return CommandController{deps: deps}
}

func builtinCommandHandlers() map[string]commandHandler {
	return map[string]commandHandler{
		"model":          (*CommandController).handleModelCommand,
		"clear":          (*CommandController).handleClearCommand,
		"fork":           (*CommandController).handleForkCommand,
		"resume":         (*CommandController).handleResumeCommand,
		"help":           (*CommandController).handleHelpCommand,
		"glob":           (*CommandController).handleGlobCommand,
		"tools":          (*CommandController).handleToolCommand,
		"plan":           (*CommandController).handlePlanCommand,
		"skills":         (*CommandController).handleSkillCommand,
		"agents":         (*CommandController).handleAgentCommand,
		"tokenlimit":     (*CommandController).handleTokenLimitCommand,
		"compact":        (*CommandController).handleCompactCommand,
		"init":           (*CommandController).handleInitCommand,
		"memory":         (*CommandController).handleMemoryCommand,
		"mcp":            (*CommandController).handleMCPCommand,
		"plugin":         (*CommandController).handlePluginCommand,
		"reload-plugins": (*CommandController).handleReloadPluginsCommand,
		"think":          (*CommandController).handleThinkCommand,
		"loop":           (*CommandController).handleLoopCommand,
		"search":         (*CommandController).handleSearchCommand,
	}
}

func (c CommandController) Execute(ctx context.Context, inputText string) (string, tea.Cmd, bool) {
	cmdName, args, isCmd := command.ParseCommand(inputText)
	if !isCmd {
		return "", nil, false
	}

	if result, followUp, handled := c.executeExitCommand(cmdName); handled {
		return result, followUp, true
	}

	if result, followUp, handled := c.executeBuiltinCommand(ctx, cmdName, args); handled {
		return result, followUp, true
	}

	if sk, ok := lookupSkill(c.deps.Skill, cmdName); ok {
		return c.executeSkillSlashCommand(sk, args), c.deps.HandleSkillInvocation(), true
	}

	if pc, ok := c.deps.Command.IsCustomCommand(cmdName); ok {
		return c.executeCustomCommand(pc, args), c.deps.HandleSkillInvocation(), true
	}

	return unknownCommandResult(cmdName), nil, true
}

func (c CommandController) HandleSubmit(inputText string) (tea.Cmd, bool) {
	preserve := shouldPreserveCommandInConversation(c.deps.Command, inputText)
	preAppended := false
	if preserve && ShouldPreserveBeforeCommandExecution(inputText) {
		c.deps.Conversation.Append(core.ChatMessage{Role: core.RoleUser, Content: inputText})
		preAppended = true
	}

	insertAt := len(c.deps.Conversation.Messages)
	result, cmd, isCmd := c.Execute(context.Background(), inputText)
	if !isCmd {
		if preAppended && len(c.deps.Conversation.Messages) > 0 {
			c.deps.Conversation.Messages = c.deps.Conversation.Messages[:len(c.deps.Conversation.Messages)-1]
		}
		return nil, false
	}

	c.deps.Input.Reset()

	if preserve && !preAppended {
		c.insertConversationMessage(insertAt, core.ChatMessage{Role: core.RoleUser, Content: inputText})
	}
	if result != "" {
		c.deps.Conversation.AddNotice(result)
	}

	cmds := c.deps.CommitMessages()
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...), true
}

func (c CommandController) executeBuiltinCommand(ctx context.Context, cmdName, args string) (string, tea.Cmd, bool) {
	handler, ok := builtinCommandHandlers()[cmdName]
	if !ok {
		return "", nil, false
	}
	result, followUp, err := handler(&c, ctx, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil, true
	}
	return result, followUp, true
}

func (c CommandController) executeExitCommand(cmdName string) (string, tea.Cmd, bool) {
	if cmdName != "exit" {
		return "", nil, false
	}
	c.deps.StopAgentSession()
	c.deps.Conversation.Stream.Stop()
	c.deps.FireSessionEnd("prompt_input_exit")
	return "", tea.Quit, true
}

func (c CommandController) executeSkillSlashCommand(sk *skill.Skill, args string) string {
	if c.deps.Skill != nil {
		c.deps.Input.Skill.PendingInstructions = c.deps.Skill.GetSkillInvocationPrompt(sk.FullName())
	}
	if c.deps.Plugin != nil {
		c.deps.Plugin.SetActivePluginRoot(c.deps.Plugin.FindPluginRootForPath(sk.SkillDir))
	}
	if args != "" {
		c.deps.Input.Skill.PendingArgs = fmt.Sprintf("/%s %s", sk.FullName(), args)
	} else {
		c.deps.Input.Skill.PendingArgs = fmt.Sprintf("/%s", sk.FullName())
	}
	return ""
}

func ApplySkillInvocation(state *Model, sk *skill.Skill, args string, skillSvc skill.Service, pluginSvc plugin.Service) {
	if skillSvc != nil {
		state.Skill.PendingInstructions = skillSvc.GetSkillInvocationPrompt(sk.FullName())
	}
	if pluginSvc != nil {
		pluginSvc.SetActivePluginRoot(pluginSvc.FindPluginRootForPath(sk.SkillDir))
	}
	if args != "" {
		state.Skill.PendingArgs = fmt.Sprintf("/%s %s", sk.FullName(), args)
	} else {
		state.Skill.PendingArgs = fmt.Sprintf("/%s", sk.FullName())
	}
}

func (c CommandController) executeCustomCommand(pc *command.CustomCommand, args string) string {
	instructions := pc.GetInstructions()
	if instructions != "" {
		c.deps.Input.Skill.PendingInstructions = fmt.Sprintf("<custom-command name=%q>\n%s\n</custom-command>", pc.FullName(), instructions)
	}
	if c.deps.Plugin != nil {
		c.deps.Plugin.SetActivePluginRoot(c.deps.Plugin.FindPluginRootForPath(pc.FilePath))
	}
	if args != "" {
		c.deps.Input.Skill.PendingArgs = fmt.Sprintf("/%s %s", pc.FullName(), args)
	} else {
		c.deps.Input.Skill.PendingArgs = fmt.Sprintf("/%s", pc.FullName())
	}
	return ""
}

func (c CommandController) insertConversationMessage(idx int, msg core.ChatMessage) {
	if idx < 0 || idx >= len(c.deps.Conversation.Messages) {
		c.deps.Conversation.Append(msg)
		return
	}
	c.deps.Conversation.Messages = append(c.deps.Conversation.Messages, core.ChatMessage{})
	copy(c.deps.Conversation.Messages[idx+1:], c.deps.Conversation.Messages[idx:])
	c.deps.Conversation.Messages[idx] = msg
	if idx < c.deps.Conversation.CommittedCount {
		c.deps.Conversation.CommittedCount++
	}
}

func (c *CommandController) handleHelpCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")
	builtins := c.deps.Command.BuiltinNames()
	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		info := builtins[name]
		fmt.Fprintf(&sb, "  /%s - %s\n", info.Name, info.Description)
	}
	pluginCmds := c.deps.Command.GetCustomCommands()
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

func (c *CommandController) handleClearCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	c.deps.StopAgentSession()
	c.deps.Conversation.Stream.Stop()
	if c.deps.Tool.Cancel != nil {
		c.deps.Tool.Cancel()
	}
	c.deps.Tool.Reset()
	c.deps.Conversation.Clear()
	c.deps.ResetTokens()
	c.deps.Tracker.Reset()
	if c.deps.ResetFetched != nil {
		c.deps.ResetFetched()
	}
	c.deps.ResetCronQueue()
	cmds := []tea.Cmd{tea.ClearScreen}
	if os.Getenv("TMUX") != "" {
		cmds = append(cmds, func() tea.Msg {
			_ = exec.Command("tmux", "clear-history").Run()
			return nil
		})
	}
	return "", tea.Batch(cmds...), nil
}

func (c CommandController) HandleClearForTests(ctx context.Context, args string) (string, tea.Cmd, error) {
	return c.handleClearCommand(ctx, args)
}

func (c *CommandController) handleForkCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	if len(c.deps.Conversation.Messages) == 0 {
		return "Nothing to fork — no messages in current session.", nil, nil
	}
	if err := c.deps.PersistSession(); err != nil {
		return "", nil, fmt.Errorf("failed to save session before fork: %w", err)
	}
	if c.deps.GetSessionID() == "" {
		return "No active session to fork.", nil, nil
	}
	originalID, err := c.deps.ForkSession()
	if err != nil {
		return "", nil, fmt.Errorf("failed to fork session: %w", err)
	}
	c.deps.InitTaskStorage()
	c.deps.ReconfigureAgentTool()
	return fmt.Sprintf("Forked conversation. You are now in the fork.\nTo resume the original: gen -r %s", originalID), nil, nil
}

func (c *CommandController) handleResumeCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	if err := c.deps.EnsureSessionStore(c.deps.Cwd); err != nil {
		return "", nil, fmt.Errorf("failed to initialize session store: %w", err)
	}
	if err := c.deps.Input.Session.Selector.EnterSelect(c.deps.Width, c.deps.Height, c.deps.GetSessionStore(), c.deps.Cwd); err != nil {
		return "", nil, fmt.Errorf("failed to open session selector: %w", err)
	}
	return "", nil, nil
}

func (c *CommandController) handleSearchCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	if err := c.deps.Input.Search.Enter(c.deps.ProviderStore, c.deps.Width, c.deps.Height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func (c *CommandController) handleModelCommand(ctx context.Context, _ string) (string, tea.Cmd, error) {
	cmd, err := c.deps.Input.Provider.Selector.Enter(ctx, c.deps.Width, c.deps.Height)
	if err != nil {
		return "", nil, err
	}
	return "", cmd, nil
}

func (c *CommandController) handleInitCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	result, err := HandleInitCommand(c.deps.Cwd, args)
	return result, nil, err
}

func (c *CommandController) handleMemoryCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	result, editPath, err := HandleMemoryCommand(&c.deps.Input.Memory.Selector, c.deps.Cwd, c.deps.Width, c.deps.Height, args)
	if err != nil {
		return "", nil, err
	}
	if editPath != "" {
		c.deps.Input.Memory.EditingFile = editPath
		return result, c.deps.StartExternalEditor(editPath), nil
	}
	return result, nil, nil
}

func (c *CommandController) handleMCPCommand(ctx context.Context, args string) (string, tea.Cmd, error) {
	result, editInfo, err := HandleMCPCommand(ctx, &c.deps.Input.MCP.Selector, c.deps.Width, c.deps.Height, args)
	if err != nil {
		return "", nil, err
	}
	if editInfo != nil {
		c.deps.Input.MCP.EditingFile = editInfo.TempFile
		c.deps.Input.MCP.EditingServer = editInfo.ServerName
		c.deps.Input.MCP.EditingScope = editInfo.Scope
		return result, StartMCPEditor(editInfo.TempFile), nil
	}
	if c.deps.Input.MCP.Selector.IsActive() {
		return result, c.deps.Input.MCP.Selector.AutoReconnect(), nil
	}
	return result, nil, nil
}

func (c *CommandController) handlePluginCommand(ctx context.Context, args string) (string, tea.Cmd, error) {
	result, err := HandlePluginCommand(ctx, &c.deps.Input.Plugin, c.deps.Cwd, c.deps.Width, c.deps.Height, args)
	return result, nil, err
}

func (c *CommandController) handleReloadPluginsCommand(ctx context.Context, args string) (string, tea.Cmd, error) {
	if strings.TrimSpace(args) != "" {
		return "Usage: /reload-plugins", nil, nil
	}
	if err := c.deps.Plugin.Load(ctx, c.deps.Cwd); err != nil {
		return "", nil, fmt.Errorf("failed to reload plugin registry: %w", err)
	}
	_ = c.deps.Plugin.LoadClaudePlugins(ctx)
	if err := c.deps.ReloadPluginBackedState(); err != nil {
		return "", nil, err
	}
	return "Reloaded plugins and refreshed plugin-backed skills, agents, MCP servers, and hooks.", nil, nil
}

func (c *CommandController) handleGlobCommand(ctx context.Context, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /glob <pattern> [path]", nil, nil
	}
	params := map[string]any{"pattern": args}
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 2 {
		params["pattern"] = parts[0]
		params["path"] = parts[1]
	}
	result := c.deps.ToolSvc.Execute(ctx, "glob", params, c.deps.Cwd)
	return conv.RenderToolResult(result, c.deps.Width), nil, nil
}

func (c *CommandController) handleToolCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	var mcpTools func() []core.ToolSchema
	if c.deps.MCP != nil {
		mcpTools = c.deps.MCP.ListTools
	}
	if err := c.deps.Input.Tool.EnterSelect(c.deps.Width, c.deps.Height, c.deps.DisabledTools, mcpTools); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func (c *CommandController) handleSkillCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	if err := c.deps.Input.Skill.Selector.EnterSelect(c.deps.Width, c.deps.Height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func (c *CommandController) handleAgentCommand(_ context.Context, _ string) (string, tea.Cmd, error) {
	if err := c.deps.Input.Agent.EnterSelect(c.deps.Width, c.deps.Height); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

func (c *CommandController) handlePlanCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	if args == "" {
		return "Usage: /plan <task description>\n\nEnter plan mode to explore the codebase and create an implementation plan before making changes.", nil, nil
	}
	if err := c.deps.EnterPlanMode(args); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Entering plan mode for: %s\n\nI will explore the codebase and create an implementation plan. Only read-only tools are available until the plan is approved.", args), nil, nil
}

func (c *CommandController) handleThinkCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(strings.ToLower(args))
	switch args {
	case "off", "0":
		c.deps.SetThinkingLevel(llm.ThinkingOff)
	case "", "toggle":
		c.deps.SetThinkingLevel(c.deps.GetThinkingLevel().Next())
	case "think", "normal", "1":
		c.deps.SetThinkingLevel(llm.ThinkingNormal)
	case "think+", "high", "2":
		c.deps.SetThinkingLevel(llm.ThinkingHigh)
	case "ultra", "ultrathink", "max", "3":
		c.deps.SetThinkingLevel(llm.ThinkingUltra)
	default:
		return "Usage: /think [off|think|think+|ultra]\n\nLevels:\n  off        — No extended thinking\n  think      — Moderate thinking budget\n  think+     — Extended thinking budget\n  ultra      — Maximum thinking budget\n\nWithout arguments, cycles to the next level.", nil, nil
	}
	c.deps.Input.Provider.StatusMessage = fmt.Sprintf("thinking: %s", c.deps.GetThinkingLevel().String())
	return "", kit.StatusTimer(3 * time.Second), nil
}

func (c *CommandController) handleLoopCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return loopUsage(), nil, nil
	}
	if result, handled, err := handleLoopAdminCommand(c.deps.Cron, args); handled {
		return result, nil, err
	}
	if strings.HasPrefix(strings.ToLower(args), "once ") {
		parsed, err := cron.ParseLoopOnceCommand(strings.TrimSpace(args[5:]), time.Now())
		if err != nil {
			return loopUsage(), nil, nil
		}
		job, err := c.deps.Cron.Create(parsed.Cron, parsed.Prompt, false, false)
		if err != nil {
			return "", nil, err
		}
		if c.deps.Conversation.Messages == nil {
			*c.deps.Conversation = conv.NewConversation()
		}
		c.deps.Conversation.AddNotice(fmt.Sprintf("Scheduled one-shot task %s (%s, cron `%s`).%s It will fire once and auto-delete.", job.ID, parsed.Human, parsed.Cron, parsed.Note))
		return "", nil, nil
	}
	parsed, err := cron.ParseLoopCommand(args, time.Now())
	if err != nil {
		return loopUsage(), nil, nil
	}
	job, err := c.deps.Cron.Create(parsed.Cron, parsed.Prompt, true, false)
	if err != nil {
		return "", nil, err
	}
	if c.deps.Conversation.Messages == nil {
		*c.deps.Conversation = conv.NewConversation()
	}
	c.deps.Conversation.AddNotice(fmt.Sprintf("Scheduled recurring task %s (%s, cron `%s`).%s Auto-expires after 7 days. Executing now.", job.ID, parsed.Human, parsed.Cron, parsed.Note))
	c.deps.Conversation.Append(core.ChatMessage{Role: core.RoleUser, Content: parsed.Prompt})
	return "", c.deps.StartProviderTurn(parsed.Prompt), nil
}

func handleLoopAdminCommand(cronSvc cron.Service, args string) (string, bool, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return "", false, nil
	}
	switch strings.ToLower(fields[0]) {
	case "list", "ls":
		return renderLoopJobList(cronSvc), true, nil
	case "delete", "del", "rm", "remove", "cancel":
		if len(fields) < 2 {
			return "Usage: /loop delete <job-id>", true, nil
		}
		if strings.EqualFold(fields[1], "all") {
			jobs := cronSvc.List()
			for _, job := range jobs {
				if err := cronSvc.Delete(job.ID); err != nil {
					return "", true, err
				}
			}
			return fmt.Sprintf("Cancelled %d scheduled task(s).", len(jobs)), true, nil
		}
		id := strings.TrimSpace(fields[1])
		if id == "" {
			return "Usage: /loop delete <job-id>", true, nil
		}
		if err := cronSvc.Delete(id); err != nil {
			return "", true, err
		}
		return fmt.Sprintf("Cancelled scheduled task %s.", id), true, nil
	default:
		return "", false, nil
	}
}

func renderLoopJobList(cronSvc cron.Service) string {
	jobs := cronSvc.List()
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

func (c *CommandController) handleTokenLimitCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	result, cmd, err := HandleTokenLimitCommand(TokenLimitDeps{
		CurrentModel: c.deps.CurrentModel,
		Provider:     c.deps.LLMProvider,
		Store:        c.deps.ProviderStore,
		InputTokens:  c.deps.InputTokens,
		Cwd:          c.deps.Cwd,
		SpinnerTick:  c.deps.SpinnerTickCmd(),
		ToolSvc:      c.deps.ToolSvc,
	}, args)
	if cmd != nil {
		c.deps.Input.Provider.FetchingLimits = true
	}
	return result, cmd, err
}

func (c *CommandController) handleCompactCommand(_ context.Context, args string) (string, tea.Cmd, error) {
	if c.deps.LLMProvider == nil {
		return "No provider connected. Use /provider to connect.", nil, nil
	}
	if len(c.deps.Conversation.Messages) == 0 {
		return "No active LLM session. Send a message first to initialize the client.", nil, nil
	}
	if len(c.deps.Conversation.Messages) < 3 {
		return "Not enough conversation history to compact.", nil, nil
	}
	if c.deps.Conversation.Stream.Active {
		return "Cannot compact while streaming.", nil, nil
	}
	c.deps.Conversation.Compact.Active = true
	c.deps.Conversation.Compact.Focus = strings.TrimSpace(args)
	c.deps.Conversation.Compact.Phase = conv.PhaseSummarizing
	return "", tea.Batch(c.deps.SpinnerTickCmd(), conv.CompactCmd(c.deps.BuildCompactRequest(c.deps.Conversation.Compact.Focus, "manual"))), nil
}

func lookupSkill(svc skill.Service, cmd string) (*skill.Skill, bool) {
	if svc == nil {
		return nil, false
	}
	sk, ok := svc.Get(cmd)
	if !ok || !sk.IsEnabled() {
		return nil, false
	}
	return sk, true
}

func LookupSkillCommand(cmd string) (*skill.Skill, bool) {
	return lookupSkill(skill.DefaultIfInit(), cmd)
}

func unknownCommandResult(cmd string) string {
	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd)
}

func SkillCommandInfos() []command.Info {
	svc := skill.DefaultIfInit()
	if svc == nil {
		return nil
	}
	enabled := svc.GetEnabled()
	infos := make([]command.Info, 0, len(enabled))
	for _, sk := range enabled {
		description := sk.Description
		if sk.ArgumentHint != "" {
			description += " " + sk.ArgumentHint
		}
		infos = append(infos, command.Info{Name: sk.FullName(), Description: description})
	}
	return infos
}

func shouldPreserveCommandInConversation(cmdSvc command.Service, inputText string) bool {
	name, _, isCmd := command.ParseCommand(inputText)
	if !isCmd {
		return false
	}
	switch name {
	case "clear", "exit":
		return false
	}
	if _, ok := LookupSkillCommand(name); ok {
		return false
	}
	if _, ok := cmdSvc.IsCustomCommand(name); ok {
		return false
	}
	return true
}

func ShouldPreserveBeforeCommandExecution(inputText string) bool {
	name, _, isCmd := command.ParseCommand(inputText)
	if !isCmd {
		return false
	}
	return name == "loop"
}

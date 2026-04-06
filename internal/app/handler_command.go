package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/command"
	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	appmcp "github.com/yanmxa/gencode/internal/app/mcp"
	appmemory "github.com/yanmxa/gencode/internal/app/memory"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

type CommandHandler func(ctx context.Context, m *model, args string) (string, tea.Cmd, error)

var builtinCommandHandlers = map[string]CommandHandler{
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
	"init":       handleInitCommand,
	"memory":     handleMemoryCommand,
	"mcp":        handleMCPCommand,
	"plugin":     handlePluginCommand,
	"think":      handleThinkCommand,
	"loop":       handleLoopCommand,
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

	return unknownCommandResult(cmd), nil, true
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) {
	if skill.DefaultRegistry != nil {
		m.skill.PendingInstructions = skill.DefaultRegistry.GetSkillInvocationPrompt(sk.FullName())
	}

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

func unknownCommandResult(cmd string) string {
	return fmt.Sprintf("Unknown command: /%s\nType /help for available commands.", cmd)
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
}

func handlePluginCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	result, err := appplugin.HandleCommand(ctx, &m.plugin.Selector, m.cwd, m.width, m.height, args)
	return result, nil, err
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

const loopDefaultInterval = "10m"

var (
	loopLeadingIntervalRe = regexp.MustCompile(`^(\d+)([smhd])$`)
	loopTrailingEveryRe   = regexp.MustCompile(`(?i)^(.*?)(?:\s+every\s+)(\d+)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)\s*$`)
	loopTrailingInRe      = regexp.MustCompile(`(?i)^(.*?)(?:\s+in\s+)(\d+)\s*(s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)\s*$`)
)

func handleLoopCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return loopUsage(), nil, nil
	}
	if result, handled, err := handleLoopAdminCommand(args); handled {
		return result, nil, err
	}
	if strings.HasPrefix(strings.ToLower(args), "once ") {
		parsed, err := parseLoopOnceCommand(strings.TrimSpace(args[5:]), time.Now())
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

	parsed, err := parseLoopCommand(args, time.Now())
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
	m.conv.Append(message.ChatMessage{
		Role:    message.RoleUser,
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

type loopCommand struct {
	Prompt string
	Cron   string
	Human  string
	Note   string
}

func parseLoopCommand(args string, now time.Time) (loopCommand, error) {
	input := strings.TrimSpace(args)
	if input == "" {
		return loopCommand{}, fmt.Errorf("empty loop command")
	}

	if matches := loopTrailingEveryRe.FindStringSubmatch(input); len(matches) == 4 {
		interval, err := normalizeLoopInterval(matches[2], matches[3])
		if err != nil {
			return loopCommand{}, err
		}
		return buildLoopCommand(strings.TrimSpace(matches[1]), interval, now)
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return loopCommand{}, fmt.Errorf("empty loop command")
	}
	if loopLeadingIntervalRe.MatchString(parts[0]) {
		return buildLoopCommand(strings.TrimSpace(strings.Join(parts[1:], " ")), parts[0], now)
	}

	return buildLoopCommand(input, loopDefaultInterval, now)
}

func buildLoopCommand(prompt, interval string, now time.Time) (loopCommand, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return loopCommand{}, fmt.Errorf("empty prompt")
	}
	cadence, err := loopIntervalToCadence(interval, now)
	if err != nil {
		return loopCommand{}, err
	}
	return loopCommand{
		Prompt: prompt,
		Cron:   cadence.Cron,
		Human:  cadence.Human,
		Note:   cadence.Note,
	}, nil
}

func parseLoopOnceCommand(args string, now time.Time) (loopCommand, error) {
	input := strings.TrimSpace(args)
	if input == "" {
		return loopCommand{}, fmt.Errorf("empty once command")
	}

	if matches := loopTrailingInRe.FindStringSubmatch(input); len(matches) == 4 {
		interval, err := normalizeLoopInterval(matches[2], matches[3])
		if err != nil {
			return loopCommand{}, err
		}
		return buildLoopOnceCommand(strings.TrimSpace(matches[1]), interval, now)
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return loopCommand{}, fmt.Errorf("empty once command")
	}
	if loopLeadingIntervalRe.MatchString(parts[0]) {
		return buildLoopOnceCommand(strings.TrimSpace(strings.Join(parts[1:], " ")), parts[0], now)
	}

	return loopCommand{}, fmt.Errorf("invalid once command")
}

func buildLoopOnceCommand(prompt, interval string, now time.Time) (loopCommand, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return loopCommand{}, fmt.Errorf("empty prompt")
	}

	spec, err := parseLoopIntervalSpec(interval)
	if err != nil {
		return loopCommand{}, err
	}
	minutes := intervalSpecToMinutes(spec)
	target := now.Add(time.Duration(minutes) * time.Minute).Truncate(time.Minute)

	note := ""
	if spec.Unit == "s" {
		note = fmt.Sprintf(" Rounded `%s` to `%s`.", spec.Requested, humanizeMinutes(minutes))
	}

	return loopCommand{
		Prompt: prompt,
		Cron:   fmt.Sprintf("%d %d %d %d *", target.Minute(), target.Hour(), target.Day(), int(target.Month())),
		Human:  fmt.Sprintf("once at %s", target.Format("2006-01-02 15:04")),
		Note:   note,
	}, nil
}

type loopCadence struct {
	Cron  string
	Human string
	Note  string
}

type loopIntervalSpec struct {
	Value     int
	Unit      string
	Requested string
}

func normalizeLoopInterval(value, unit string) (string, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return "", fmt.Errorf("invalid interval")
	}
	switch strings.ToLower(unit) {
	case "s", "sec", "secs", "second", "seconds":
		return fmt.Sprintf("%ds", n), nil
	case "m", "min", "mins", "minute", "minutes":
		return fmt.Sprintf("%dm", n), nil
	case "h", "hr", "hrs", "hour", "hours":
		return fmt.Sprintf("%dh", n), nil
	case "d", "day", "days":
		return fmt.Sprintf("%dd", n), nil
	default:
		return "", fmt.Errorf("invalid interval unit")
	}
}

func parseLoopIntervalSpec(interval string) (loopIntervalSpec, error) {
	matches := loopLeadingIntervalRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(interval)))
	if len(matches) != 3 {
		return loopIntervalSpec{}, fmt.Errorf("invalid interval")
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return loopIntervalSpec{}, fmt.Errorf("invalid interval")
	}

	return loopIntervalSpec{
		Value:     value,
		Unit:      matches[2],
		Requested: matches[0],
	}, nil
}

func loopIntervalToCadence(interval string, now time.Time) (loopCadence, error) {
	spec, err := parseLoopIntervalSpec(interval)
	if err != nil {
		return loopCadence{}, err
	}

	requestedMinutes := intervalSpecToMinutes(spec)
	effectiveMinutes := requestedMinutes
	switch {
	case requestedMinutes < 60:
		effectiveMinutes = nearestAllowed(requestedMinutes, []int{1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30, 60})
	case requestedMinutes < 24*60:
		effectiveMinutes = nearestAllowed(requestedMinutes, []int{60, 120, 180, 240, 360, 480, 720, 1440})
	default:
		var allowed []int
		for d := 1; d <= 31; d++ {
			allowed = append(allowed, d*24*60)
		}
		effectiveMinutes = nearestAllowed(requestedMinutes, allowed)
	}

	cadence := cadenceFromMinutes(effectiveMinutes, now)
	if requestedMinutes != effectiveMinutes {
		cadence.Note = fmt.Sprintf(" Rounded `%s` to `%s`.", spec.Requested, humanizeMinutes(effectiveMinutes))
	}
	if spec.Unit == "s" && requestedMinutes == effectiveMinutes {
		cadence.Note = fmt.Sprintf(" Rounded `%s` to `%s`.", spec.Requested, humanizeMinutes(effectiveMinutes))
	}
	return cadence, nil
}

func intervalSpecToMinutes(spec loopIntervalSpec) int {
	switch spec.Unit {
	case "s":
		minutes := (spec.Value + 59) / 60
		if minutes < 1 {
			minutes = 1
		}
		return minutes
	case "m":
		return spec.Value
	case "h":
		return spec.Value * 60
	case "d":
		return spec.Value * 24 * 60
	default:
		return spec.Value
	}
}

func nearestAllowed(requested int, allowed []int) int {
	best := allowed[0]
	bestDelta := absInt(requested - best)
	for _, candidate := range allowed[1:] {
		delta := absInt(requested - candidate)
		if delta < bestDelta || (delta == bestDelta && candidate > best) {
			best = candidate
			bestDelta = delta
		}
	}
	return best
}

func cadenceFromMinutes(totalMinutes int, now time.Time) loopCadence {
	if totalMinutes < 60 {
		return loopCadence{
			Cron:  fmt.Sprintf("*/%d * * * *", totalMinutes),
			Human: humanizeMinutes(totalMinutes),
		}
	}

	minute := chooseLoopScheduleMinute(now.Minute())
	if totalMinutes < 24*60 {
		hours := totalMinutes / 60
		hourField := "*"
		if hours > 1 {
			start := now.Hour() % hours
			if start == 0 {
				hourField = fmt.Sprintf("*/%d", hours)
			} else {
				hourField = fmt.Sprintf("%d-23/%d", start, hours)
			}
		}
		return loopCadence{
			Cron:  fmt.Sprintf("%d %s * * *", minute, hourField),
			Human: humanizeMinutes(totalMinutes),
		}
	}

	days := totalMinutes / (24 * 60)
	hour := now.Hour()
	if days <= 1 {
		return loopCadence{
			Cron:  fmt.Sprintf("%d %d * * *", minute, hour),
			Human: humanizeMinutes(totalMinutes),
		}
	}
	return loopCadence{
		Cron:  fmt.Sprintf("%d %d */%d * *", minute, hour, days),
		Human: humanizeMinutes(totalMinutes),
	}
}

func chooseLoopScheduleMinute(minute int) int {
	switch minute {
	case 0:
		return 7
	case 30:
		return 37
	default:
		return minute
	}
}

func humanizeMinutes(totalMinutes int) string {
	switch {
	case totalMinutes < 60:
		return fmt.Sprintf("every %d minute(s)", totalMinutes)
	case totalMinutes%(24*60) == 0:
		return fmt.Sprintf("every %d day(s)", totalMinutes/(24*60))
	default:
		return fmt.Sprintf("every %d hour(s)", totalMinutes/60)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func loopUsage() string {
	return "Usage: /loop [interval] <prompt>\n       /loop once <interval> <prompt>\n       /loop once <prompt> in <interval>\n       /loop list\n       /loop delete <job-id>\n       /loop delete all\nExamples: /loop 5m check the deploy, /loop check the deploy every 20m, /loop once 20m check the deploy"
}

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	m.conv.Clear()
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0
	tool.DefaultTodoStore.Reset()
	tool.ResetFetched()
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

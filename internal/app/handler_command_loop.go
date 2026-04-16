// handler_command_loop.go contains the /loop command handler and all supporting
// loop scheduling logic (parsing, cron generation, admin sub-commands).
package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/core"
)

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

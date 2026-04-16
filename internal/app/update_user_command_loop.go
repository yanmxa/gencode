package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconv "github.com/yanmxa/gencode/internal/app/output/conversation"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/cron"
)

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

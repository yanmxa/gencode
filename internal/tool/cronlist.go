package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/tool/ui"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

// CronListTool lists all scheduled cron jobs.
type CronListTool struct{}

func (t *CronListTool) Name() string        { return "CronList" }
func (t *CronListTool) Description() string { return "List all scheduled cron jobs" }
func (t *CronListTool) Icon() string        { return "clock" }

func (t *CronListTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	jobs := cron.DefaultStore.List()
	if len(jobs) == 0 {
		return ui.ToolResult{
			Success: true,
			Output:  "No scheduled jobs.",
			Metadata: ui.ResultMetadata{
				Title: t.Name(),
				Icon:  t.Icon(),
			},
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d scheduled job(s):\n\n", len(jobs)))
	for _, j := range jobs {
		desc := cron.Describe(j.Cron)
		mode := "recurring"
		if !j.Recurring {
			mode = "one-shot"
		}
		if j.Durable {
			mode += ", durable"
		}
		sb.WriteString(fmt.Sprintf("  %s  %s (%s)\n", j.ID, desc, mode))
		sb.WriteString(fmt.Sprintf("    Prompt: %s\n", shared.TruncateText(j.Prompt, 80)))
		sb.WriteString(fmt.Sprintf("    Next:   %s\n", j.NextFire.Format("15:04:05")))
		if j.FiredCount > 0 {
			sb.WriteString(fmt.Sprintf("    Fired:  %d times (last: %s)\n", j.FiredCount, j.LastFired.Format("15:04:05")))
		}
		sb.WriteString("\n")
	}

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%d job(s)", len(jobs)),
		},
	}
}

func init() {
	Register(&CronListTool{})
}

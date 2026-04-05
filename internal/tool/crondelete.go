package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// CronDeleteTool cancels a scheduled cron job.
type CronDeleteTool struct{}

func (t *CronDeleteTool) Name() string        { return "CronDelete" }
func (t *CronDeleteTool) Description() string { return "Cancel a scheduled cron job" }
func (t *CronDeleteTool) Icon() string        { return "clock" }

func (t *CronDeleteTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	id := getString(params, "id")
	if id == "" {
		return ui.NewErrorResult(t.Name(), "job id is required")
	}

	if err := cron.DefaultStore.Delete(id); err != nil {
		return ui.NewErrorResult(t.Name(), err.Error())
	}

	return ui.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Cancelled cron job %s.", id),
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("Cancelled %s", id),
		},
	}
}

func init() {
	Register(&CronDeleteTool{})
}

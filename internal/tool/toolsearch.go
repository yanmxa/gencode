package tool

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// ToolSearchTool fetches full schema definitions for deferred tools.
type ToolSearchTool struct{}

func (t *ToolSearchTool) Name() string        { return "ToolSearch" }
func (t *ToolSearchTool) Description() string { return "Fetch schemas for deferred tools" }
func (t *ToolSearchTool) Icon() string        { return "search" }

func (t *ToolSearchTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	query := getString(params, "query")
	if query == "" {
		return ui.NewErrorResult(t.Name(), "query is required")
	}

	maxResults := getInt(params, "max_results", 5)

	matched := SearchDeferredTools(query, maxResults)

	// Mark matched tools as fetched so they appear in subsequent tool sets
	for _, tool := range matched {
		MarkFetched(tool.Name)
	}

	output := FormatToolSchemas(matched)

	subtitle := fmt.Sprintf("%d tool(s) found", len(matched))
	if len(matched) == 0 {
		subtitle = "no matches"
	}

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: subtitle,
		},
	}
}

func init() {
	Register(&ToolSearchTool{})
}

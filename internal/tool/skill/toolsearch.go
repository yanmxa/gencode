package skill

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// ToolSearchTool fetches full schema definitions for deferred tools.
type ToolSearchTool struct{}

func (t *ToolSearchTool) Name() string        { return "ToolSearch" }
func (t *ToolSearchTool) Description() string { return "Fetch schemas for deferred tools" }
func (t *ToolSearchTool) Icon() string        { return "search" }

func (t *ToolSearchTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	query := tool.GetString(params, "query")
	if query == "" {
		return toolresult.NewErrorResult(t.Name(), "query is required")
	}

	maxResults := tool.GetInt(params, "max_results", 5)

	matched := tool.SearchDeferredTools(query, maxResults)

	// Mark matched tools as fetched so they appear in subsequent tool sets
	for _, ts := range matched {
		tool.MarkFetched(ts.Name)
	}

	output := tool.FormatToolSchemas(matched)

	subtitle := fmt.Sprintf("%d tool(s) found", len(matched))
	if len(matched) == 0 {
		subtitle = "no matches"
	}

	return toolresult.ToolResult{
		Success: true,
		Output:  output,
		Metadata: toolresult.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: subtitle,
		},
	}
}

func init() {
	tool.Register(&ToolSearchTool{})
}

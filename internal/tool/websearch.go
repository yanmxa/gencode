package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

// WebSearchTool searches the web for information
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string        { return "WebSearch" }
func (t *WebSearchTool) Description() string { return "Search the web for up-to-date information" }
func (t *WebSearchTool) Icon() string        { return ui.IconSearch }

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get query parameter (required)
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return ui.NewErrorResult(t.Name(), "query is required")
	}

	// Get optional num_results parameter
	numResults := 10
	if n, ok := params["num_results"].(float64); ok && n > 0 {
		numResults = int(n)
	}

	// Get optional domain filters
	var allowedDomains, blockedDomains []string
	if domains, ok := params["allowed_domains"].([]any); ok {
		for _, d := range domains {
			if s, ok := d.(string); ok {
				allowedDomains = append(allowedDomains, s)
			}
		}
	}
	if domains, ok := params["blocked_domains"].([]any); ok {
		for _, d := range domains {
			if s, ok := d.(string); ok {
				blockedDomains = append(blockedDomains, s)
			}
		}
	}

	// Get the configured search provider from store
	var searchProvider search.Provider
	store, err := provider.NewStore()
	if err == nil {
		providerName := store.GetSearchProvider()
		if providerName != "" {
			searchProvider = search.CreateProvider(search.ProviderName(providerName))
		}
	}

	// Use default provider if none configured
	if searchProvider == nil {
		searchProvider = search.GetDefaultProvider()
	}

	// Execute search
	opts := search.SearchOptions{
		NumResults:     numResults,
		AllowedDomains: allowedDomains,
		BlockedDomains: blockedDomains,
		Timeout:        30 * time.Second,
	}

	results, err := searchProvider.Search(ctx, query, opts)
	if err != nil {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("search failed: %v", err))
	}

	// Format results as Markdown
	var sb strings.Builder
	if len(results) == 0 {
		sb.WriteString("No results found for: " + query)
	} else {
		sb.WriteString(fmt.Sprintf("Found %d results for: %s\n\n", len(results), query))
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("- [%s](%s)\n", r.Title, r.URL))
			if r.Snippet != "" {
				sb.WriteString(fmt.Sprintf("  %s\n\n", r.Snippet))
			}
		}
	}

	duration := time.Since(start)

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:     t.Name(),
			Icon:      t.Icon(),
			Subtitle:  fmt.Sprintf("%s via %s", query, searchProvider.DisplayName()),
			ItemCount: len(results),
			Duration:  duration,
		},
	}
}

func init() {
	Register(&WebSearchTool{})
}

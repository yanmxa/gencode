package tool

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/yanmxa/gencode/internal/provider"
)

// DeferredToolNames lists tools whose schemas are NOT sent to the LLM initially.
// They appear as names in <available-deferred-tools> in the system prompt.
// The LLM calls ToolSearch to fetch their full schemas on demand.
//
// Only defer tools that are genuinely rare. Do NOT defer tools that are:
// - Needed reactively (TaskOutput/TaskStop — triggered by background completions)
// - Commonly used (EnterPlanMode — triggered by task complexity)
var DeferredToolNames = map[string]bool{
	ToolCronCreate:    true,
	ToolCronDelete:    true,
	ToolCronList:      true,
	ToolEnterWorktree: true,
	ToolExitWorktree:  true,
}

// fetchedDeferred tracks which deferred tools have been activated via ToolSearch.
var fetchedDeferred sync.Map

// MarkFetched marks a deferred tool as activated so it appears in subsequent tool sets.
func MarkFetched(name string) {
	fetchedDeferred.Store(strings.ToLower(name), true)
}

// IsFetched returns true if a deferred tool has been activated via ToolSearch.
func IsFetched(name string) bool {
	_, ok := fetchedDeferred.Load(strings.ToLower(name))
	return ok
}

// ResetFetched clears all fetched deferred tools (for new sessions).
func ResetFetched() {
	fetchedDeferred = sync.Map{}
}

// IsDeferred returns true if the tool name is in the deferred set.
func IsDeferred(name string) bool {
	return DeferredToolNames[name]
}

// DeferredToolList returns sorted deferred tool names for the system prompt.
func DeferredToolList() []string {
	names := make([]string, 0, len(DeferredToolNames))
	for name := range DeferredToolNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FormatDeferredToolsPrompt returns the <available-deferred-tools> section for the system prompt.
func FormatDeferredToolsPrompt() string {
	names := DeferredToolList()
	if len(names) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<available-deferred-tools>\n")
	for _, name := range names {
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	sb.WriteString("</available-deferred-tools>")
	return sb.String()
}

// SearchDeferredTools matches a query against deferred tool schemas.
// Supports "select:Name1,Name2" for exact match or keyword search.
// Returns matched schemas as provider.Tool slices.
func SearchDeferredTools(query string, maxResults int) []provider.Tool {
	if maxResults <= 0 {
		maxResults = 5
	}

	// All deferred tool schemas (fetched from the full schema list)
	allSchemas := allDeferredSchemas()

	// Handle "select:Name1,Name2" syntax
	if after, ok := strings.CutPrefix(query, "select:"); ok {
		names := strings.Split(after, ",")
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[strings.TrimSpace(n)] = true
		}
		var matched []provider.Tool
		for _, s := range allSchemas {
			if nameSet[s.Name] {
				matched = append(matched, s)
			}
		}
		return matched
	}

	// Keyword search: match against name and description
	queryLower := strings.ToLower(query)
	keywords := strings.Fields(queryLower)

	type scored struct {
		tool  provider.Tool
		score int
	}
	var results []scored

	for _, s := range allSchemas {
		score := 0
		nameLower := strings.ToLower(s.Name)
		descLower := strings.ToLower(s.Description)

		for _, kw := range keywords {
			if strings.Contains(nameLower, kw) {
				score += 10 // name match is worth more
			}
			if strings.Contains(descLower, kw) {
				score += 1
			}
		}
		if score > 0 {
			results = append(results, scored{tool: s, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	matched := make([]provider.Tool, 0, maxResults)
	for i, r := range results {
		if i >= maxResults {
			break
		}
		matched = append(matched, r.tool)
	}
	return matched
}

// FormatToolSchemas formats tool schemas as a <functions> block (matching CC's format).
func FormatToolSchemas(tools []provider.Tool) string {
	if len(tools) == 0 {
		return "No matching tools found."
	}

	var sb strings.Builder
	sb.WriteString("<functions>\n")
	for _, t := range tools {
		entry := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		}
		data, _ := json.Marshal(entry)
		sb.WriteString("<function>")
		sb.Write(data)
		sb.WriteString("</function>\n")
	}
	sb.WriteString("</functions>")
	return sb.String()
}

// allDeferredSchemas returns the full schemas for all deferred tools.
func allDeferredSchemas() []provider.Tool {
	// Collect from known schema slices that contain deferred tools
	var all []provider.Tool
	for _, s := range CronToolSchemas {
		if DeferredToolNames[s.Name] {
			all = append(all, s)
		}
	}
	for _, s := range WorktreeToolSchemas {
		if DeferredToolNames[s.Name] {
			all = append(all, s)
		}
	}
	return all
}

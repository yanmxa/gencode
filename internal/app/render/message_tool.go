package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
)

// ToolCallsParams holds the parameters for rendering tool calls.
type ToolCallsParams struct {
	ToolCalls         []message.ToolCall
	ToolCallsExpanded bool
	ResultMap         map[string]ToolResultData
	ParallelMode      bool
	ParallelResults   map[int]bool
	TaskProgress      map[int][]string
	PendingCalls      []message.ToolCall
	SpinnerView       string
	TaskOwnerMap      map[string]string
	MDRenderer        *MDRenderer
}

// ToolResultData holds the data needed to render a tool result inline.
type ToolResultData struct {
	ToolName  string
	Content   string
	IsError   bool
	Expanded  bool
	ToolInput string
}

// RenderToolCalls renders the tool calls section of an assistant message.
func RenderToolCalls(params ToolCallsParams) string {
	var sb strings.Builder

	for _, tc := range params.ToolCalls {
		switch tc.Name {
		case tool.ToolTaskList, tool.ToolTaskCreate, tool.ToolTaskUpdate:
			continue
		}
		if tc.Name == tool.ToolAgent {
			label := FormatAgentLabel(tc.Input)
			_, hasResult := params.ResultMap[tc.ID]
			if hasResult {
				sb.WriteString(renderToolLine(label) + "\n")
			} else {
				sb.WriteString(ToolCallStyle.Render(fmt.Sprintf("%s %s", params.SpinnerView, label)))
				if !params.ToolCallsExpanded {
					sb.WriteString(ThinkingStyle.Render("  (ctrl+o to expand)"))
				}
				sb.WriteString("\n")
			}
			if params.ToolCallsExpanded && !hasResult {
				sb.WriteString(formatAgentDefinition(tc.Input))
			}
		} else if params.ToolCallsExpanded {
			toolLine := renderToolLine(tc.Name)
			sb.WriteString(toolLine + "\n")
			var p map[string]any
			if err := json.Unmarshal([]byte(tc.Input), &p); err == nil {
				for k, v := range p {
					if s, ok := v.(string); ok {
						if len(s) > 80 {
							sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("%s:", k)) + "\n")
							sb.WriteString(ToolResultExpandedStyle.Render(s) + "\n")
						} else {
							sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("%s: %s", k, s)) + "\n")
						}
					}
				}
			}
		} else {
			if tc.Name == tool.ToolTaskGet && params.TaskOwnerMap != nil {
				args := extractTaskGetDisplay(tc.Input, params.TaskOwnerMap)
				sb.WriteString(renderToolLine(fmt.Sprintf("%s(%s)", tc.Name, args)) + "\n")
			} else {
				args := ExtractToolArgs(tc.Input)
				sb.WriteString(renderToolLine(fmt.Sprintf("%s(%s)", tc.Name, args)) + "\n")
			}
		}

		if resultData, ok := params.ResultMap[tc.ID]; ok {
			resultData.ToolInput = tc.Input
			sb.WriteString(RenderToolResultInline(resultData, params.MDRenderer))
		} else if params.ParallelMode && tc.Name == tool.ToolAgent {
			sb.WriteString(RenderTaskProgressInline(tc, params.PendingCalls, params.ParallelResults, params.TaskProgress))
		}
	}

	return sb.String()
}

// RenderToolResultInline renders a tool result inline (without leading newline).
func RenderToolResultInline(data ToolResultData, mdRenderer *MDRenderer) string {
	toolName := data.ToolName
	if toolName == "" {
		toolName = "Tool"
	}

	if toolName == tool.ToolSkill {
		return RenderSkillResultInline(data)
	}
	if toolName == tool.ToolAgent {
		return RenderTaskResultInline(data, mdRenderer)
	}
	if toolName == tool.ToolTaskOutput {
		return RenderTaskOutputResultInline(data)
	}

	sizeInfo := FormatToolResultSize(toolName, data.Content)
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	summary := ToolResultStyle.Render(fmt.Sprintf("  %s  %s → %s", icon, toolName, sizeInfo))
	sb.WriteString(summary + "\n")

	if data.Expanded || data.IsError {
		for line := range strings.SplitSeq(data.Content, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// RenderSkillResultInline renders a skill result with clean formatting.
func RenderSkillResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	if data.IsError {
		summary := ToolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, data.Content))
		sb.WriteString(summary + "\n")
		return sb.String()
	}

	skillName, scriptCount, refCount := ParseSkillResultContent(data.Content)

	var resources []string
	if scriptCount > 0 {
		if scriptCount == 1 {
			resources = append(resources, "1 script")
		} else {
			resources = append(resources, fmt.Sprintf("%d scripts", scriptCount))
		}
	}
	if refCount > 0 {
		if refCount == 1 {
			resources = append(resources, "1 ref")
		} else {
			resources = append(resources, fmt.Sprintf("%d refs", refCount))
		}
	}

	result := fmt.Sprintf("Loaded: %s", skillName)
	if len(resources) > 0 {
		result += fmt.Sprintf(" [%s]", strings.Join(resources, ", "))
	}

	summary := ToolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, result))
	sb.WriteString(summary + "\n")

	if data.Expanded {
		for line := range strings.SplitSeq(data.Content, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// RenderTaskResultInline renders a Task tool result with agent-specific formatting.
func RenderTaskResultInline(data ToolResultData, mdRenderer *MDRenderer) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content

	if data.IsError {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  Agent → Error", icon)) + "\n")
		sb.WriteString(ToolResultExpandedStyle.Render("    "+content) + "\n")
		return sb.String()
	}

	taskID := ExtractField(content, "Task ID: ", "")
	isBackground := strings.Contains(content, "started in background")

	if isBackground && taskID != "" {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  → background (Task ID: %s)", icon, taskID)) + "\n")
		return sb.String()
	}

	toolUses := ExtractIntField(content, "ToolUses: ")
	tokens := ExtractIntField(content, "Tokens: ")
	duration := ExtractField(content, "Duration: ", "")
	resultModel := ExtractField(content, "Model: ", "\n")
	doneStats := buildDoneStats(toolUses, tokens, duration, resultModel)

	if !data.Expanded {
		resultLine := fmt.Sprintf("  %s  Done", icon)
		if doneStats != "" {
			resultLine += " (" + doneStats + ")"
		}
		sb.WriteString(ToolResultStyle.Render(resultLine))
		sb.WriteString(ThinkingStyle.Render("  (ctrl+o to expand)") + "\n")
		return sb.String()
	}

	if data.ToolInput != "" {
		sb.WriteString(formatAgentDefinition(data.ToolInput))
	}

	body := ""
	if _, rest, found := strings.Cut(content, "\n\n"); found {
		body = rest
	}

	processCount := ExtractIntField(content, "Process: ")
	process, response := splitByProcessCount(body, processCount)

	if process != "" {
		for line := range strings.SplitSeq(process, "\n") {
			sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  ⎿  %s", line)) + "\n")
		}
	}

	if response != "" {
		sb.WriteString(AgentLabelStyle.Render("  ⎿  Response:") + "\n")
		rendered := response
		if mdRenderer != nil {
			narrowRenderer := NewMDRenderer(mdRenderer.width - len(agentContentIndent))
			if md, err := narrowRenderer.Render(response); err == nil {
				rendered = strings.TrimSpace(md)
			}
		}
		for line := range strings.SplitSeq(rendered, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render(agentContentIndent+line) + "\n")
		}
	}

	resultLine := "  ⎿  Done"
	if doneStats != "" {
		resultLine += " (" + doneStats + ")"
	}
	sb.WriteString(ToolResultStyle.Render(resultLine) + "\n")

	return sb.String()
}

// stripMarkdownHeading removes leading `#` markers from markdown headings.
func stripMarkdownHeading(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return line
	}
	stripped := strings.TrimLeft(trimmed, "#")
	stripped = strings.TrimPrefix(stripped, " ")
	indent := line[:len(line)-len(trimmed)]
	return indent + stripped
}

// splitByProcessCount splits body into process lines and response using a known line count.
func splitByProcessCount(body string, processCount int) (process, response string) {
	if body == "" {
		return "", ""
	}
	if processCount <= 0 {
		return "", strings.TrimSpace(body)
	}

	lines := strings.SplitN(body, "\n", processCount+1)
	if len(lines) <= processCount {
		return strings.TrimSpace(strings.Join(lines, "\n")), ""
	}
	processLines := lines[:processCount]
	rest := lines[processCount]
	return strings.TrimSpace(strings.Join(processLines, "\n")), strings.TrimSpace(rest)
}

// RenderTaskOutputResultInline renders a TaskOutput result with task-specific formatting.
func RenderTaskOutputResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content

	if data.IsError {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → Error", icon)) + "\n")
		if content != "" {
			sb.WriteString(ToolResultExpandedStyle.Render("    "+content) + "\n")
		}
		return sb.String()
	}

	agentName := ExtractField(content, "Agent: ", "")
	status := ExtractField(content, "Status: ", "")
	turns := ExtractIntField(content, "Turns: ")

	var info []string
	if agentName != "" {
		info = append(info, agentName)
	}
	if status != "" {
		info = append(info, status)
	}
	if turns > 0 {
		info = append(info, fmt.Sprintf("%d turns", turns))
	}

	summaryText := "completed"
	if len(info) > 0 {
		summaryText = strings.Join(info, ", ")
	}

	sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → %s", icon, summaryText)) + "\n")

	if _, outputContent, found := strings.Cut(content, "Output:\n"); found {
		outputLines := strings.Split(outputContent, "\n")
		maxLines := 10
		if data.Expanded {
			maxLines = len(outputLines)
		}

		for i, line := range outputLines {
			if i >= maxLines {
				sb.WriteString(ToolResultExpandedStyle.Render("    ...") + "\n")
				break
			}
			sb.WriteString(ToolResultExpandedStyle.Render("    "+stripMarkdownHeading(line)) + "\n")
		}
	}

	return sb.String()
}

// ParseSkillResultContent extracts skill info from skill-invocation content.
func ParseSkillResultContent(content string) (skillName string, scriptCount, refCount int) {
	skillName = "skill"
	if idx := strings.Index(content, `<skill-invocation name="`); idx != -1 {
		start := idx + len(`<skill-invocation name="`)
		if end := strings.Index(content[start:], `"`); end != -1 {
			skillName = content[start : start+end]
		}
	}

	if idx := strings.Index(content, "Available scripts"); idx != -1 {
		section := content[idx:]
		lines := strings.Split(section, "\n")
		for i := 1; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "  - ") {
				scriptCount++
			} else if line == "" || !strings.HasPrefix(line, " ") {
				break
			}
		}
	}

	if idx := strings.Index(content, "Reference files"); idx != -1 {
		section := content[idx:]
		lines := strings.Split(section, "\n")
		for i := 1; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "  - ") {
				refCount++
			} else if line == "" || !strings.HasPrefix(line, " ") {
				break
			}
		}
	}

	return skillName, scriptCount, refCount
}

// ExtractField extracts a field value from content by prefix, returning defaultVal if not found.
func ExtractField(content, prefix, defaultVal string) string {
	idx := strings.Index(content, prefix)
	if idx == -1 {
		return defaultVal
	}
	start := idx + len(prefix)
	end := strings.Index(content[start:], "\n")
	if end == -1 {
		return content[start:]
	}
	return content[start : start+end]
}

// ExtractIntField extracts an integer field value from content by prefix.
func ExtractIntField(content, prefix string) int {
	val := ExtractField(content, prefix, "")
	if val == "" {
		return 0
	}
	end := 0
	for end < len(val) && val[end] >= '0' && val[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(val[:end])
	return n
}

// formatAgentDefinition renders the agent definition block for expanded view.
func formatAgentDefinition(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}

	var sb strings.Builder
	var meta []string
	if mode, ok := params["mode"].(string); ok && mode != "" {
		meta = append(meta, fmt.Sprintf("mode=%s", mode))
	}
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		meta = append(meta, "background")
	}
	if len(meta) > 0 {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  ⎿  [%s]", strings.Join(meta, ", "))) + "\n")
	}

	if prompt, ok := params["prompt"].(string); ok && prompt != "" {
		sb.WriteString(AgentLabelStyle.Render("  ⎿  Prompt:") + "\n")
		for line := range strings.SplitSeq(prompt, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render(agentContentIndent+line) + "\n")
		}
	}

	return sb.String()
}

// buildDoneStats builds the stats string for the Done line.
func buildDoneStats(toolUses, tokens int, duration, model string) string {
	var stats []string
	if toolUses == 1 {
		stats = append(stats, "1 tool use")
	} else if toolUses > 1 {
		stats = append(stats, fmt.Sprintf("%d tool uses", toolUses))
	}
	if tokens > 0 {
		stats = append(stats, FormatTokenCount(tokens)+" tokens")
	}
	if duration != "" {
		stats = append(stats, duration)
	}
	if model != "" {
		stats = append(stats, model)
	}
	return strings.Join(stats, " · ")
}

// FormatAgentLabel formats an Agent tool call as "AgentType: description".
func FormatAgentLabel(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "Agent"
	}

	agentType := "Agent"
	if a, ok := params["subagent_type"].(string); ok && a != "" {
		agentType = a
	}

	desc := ""
	if d, ok := params["description"].(string); ok {
		desc = d
	} else if p, ok := params["prompt"].(string); ok {
		desc = p
		if len(desc) > 40 {
			desc = desc[:40] + "..."
		}
	}

	if desc != "" {
		return fmt.Sprintf("%s: %s", agentType, desc)
	}
	return agentType
}

// extractTaskGetDisplay returns owner name for a TaskGet call if available.
func extractTaskGetDisplay(input string, ownerMap map[string]string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}
	id, _ := params["taskId"].(string)
	if owner, ok := ownerMap[id]; ok && owner != "" {
		return owner
	}
	return id
}

// ExtractToolArgs extracts the most relevant argument from a tool call input JSON.
func ExtractToolArgs(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}

	if fp, ok := params["file_path"].(string); ok {
		return fp
	}
	if c, ok := params["command"].(string); ok {
		if len(c) > 60 {
			return c[:60] + "..."
		}
		return c
	}
	if p, ok := params["pattern"].(string); ok {
		return p
	}
	if p, ok := params["path"].(string); ok {
		return p
	}
	if u, ok := params["url"].(string); ok {
		return u
	}
	if s, ok := params["skill"].(string); ok {
		return s
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := params[k].(string); ok {
			return s
		}
	}
	return ""
}

// FormatToolResultSize returns a human-readable size description for a tool result.
func FormatToolResultSize(toolName, content string) string {
	switch toolName {
	case "WebFetch":
		return formatByteSize(len(content))
	case "Write", "Edit":
		return extractParenContent(content, "completed")
	default:
		return formatLineCount(content)
	}
}

// formatByteSize formats a byte count as human-readable size.
func formatByteSize(size int) string {
	const (
		KB = 1024
		MB = KB * 1024
	)
	switch {
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

// extractParenContent extracts content between first ( and ), or returns fallback.
func extractParenContent(s, fallback string) string {
	start := strings.Index(s, "(")
	if start == -1 {
		return fallback
	}
	end := strings.Index(s[start:], ")")
	if end == -1 {
		return fallback
	}
	return s[start+1 : start+end]
}

// formatLineCount returns a line count string for the given content.
func formatLineCount(content string) string {
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return "0 lines"
	}
	lineCount := strings.Count(trimmed, "\n") + 1
	return fmt.Sprintf("%d lines", lineCount)
}

// renderToolLine renders a tool call line with a bullet icon.
func renderToolLine(label string) string {
	icon := ToolCallStyle.Render("● ")
	return lipgloss.JoinHorizontal(lipgloss.Top, icon, ToolCallStyle.Render(label))
}

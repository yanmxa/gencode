package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

// RenderTaskOutputResultInline renders a TaskOutput result with task-specific formatting.
func RenderTaskOutputResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content
	errorText := data.Error
	if errorText == "" {
		errorText = content
	}

	if data.IsError {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → Error", icon)) + "\n")
		if errorText != "" {
			sb.WriteString(ToolResultExpandedStyle.Render("    "+errorText) + "\n")
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

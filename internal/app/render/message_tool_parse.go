package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

// FormatAgentLabel formats an Agent tool call as "Agent: AgentType description".
func FormatAgentLabel(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "Agent"
	}

	agentType := ""
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

	if agentType == "" {
		if desc != "" {
			return fmt.Sprintf("Agent: %s", desc)
		}
		return "Agent"
	}
	if desc != "" {
		return fmt.Sprintf("Agent: %s %s", agentType, desc)
	}
	return fmt.Sprintf("Agent: %s", agentType)
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
	if qs, ok := params["questions"].([]any); ok {
		count := len(qs)
		if count == 1 {
			return "1 question"
		}
		return fmt.Sprintf("%d questions", count)
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
		return "no output"
	}
	lineCount := strings.Count(trimmed, "\n") + 1
	return fmt.Sprintf("%d lines", lineCount)
}

// renderToolLine renders a tool call line with a bullet icon.
func renderToolLine(label string, width int) string {
	return renderToolLineWithIcon(label, width, "●")
}

func renderToolLineWithIcon(label string, width int, iconText string) string {
	icon := ToolCallStyle.Width(2).Render(iconText)
	return lipgloss.JoinHorizontal(lipgloss.Top, icon, ToolCallStyle.Render(truncateToolLabel(label, width)))
}

func truncateToolLabel(label string, width int) string {
	maxWidth := maxToolLabelWidth(width)
	if lipgloss.Width(label) <= maxWidth {
		return label
	}
	return TruncateText(label, maxWidth)
}

func maxToolLabelWidth(width int) int {
	if width <= 0 {
		return 80
	}
	maxWidth := width * 80 / 100
	if maxWidth < 50 {
		maxWidth = 50
	}
	labelWidth := maxWidth - lipgloss.Width("● ")
	if labelWidth < 20 {
		return 20
	}
	return labelWidth
}

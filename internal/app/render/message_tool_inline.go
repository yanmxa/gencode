package render

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/tool"
)

// RenderToolResultInline renders a tool result inline (without leading newline).
func RenderToolResultInline(data ToolResultData, mdRenderer *MDRenderer) string {
	toolName := data.ToolName
	if toolName == "" {
		toolName = "Tool"
	}

	switch toolName {
	case tool.ToolSkill:
		return RenderSkillResultInline(data)
	case tool.ToolAgent:
		return RenderTaskResultInline(data, mdRenderer)
	case tool.ToolTaskOutput:
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

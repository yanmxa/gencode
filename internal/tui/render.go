package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)


func createMarkdownRenderer(width int) *glamour.TermRenderer {
	wrapWidth := max(width-4, minWrapWidth)

	var compactStyle ansi.StyleConfig
	if lipgloss.HasDarkBackground() {
		compactStyle = styles.DarkStyleConfig
	} else {
		compactStyle = styles.LightStyleConfig
	}

	uintPtr := func(u uint) *uint { return &u }
	compactStyle.Document.Margin = uintPtr(0)
	compactStyle.Paragraph.Margin = uintPtr(0)
	compactStyle.CodeBlock.Margin = uintPtr(0)

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(compactStyle),
		glamour.WithWordWrap(wrapWidth),
	)
	return renderer
}

func (m model) renderWelcome() string {
	gradient := []lipgloss.Color{
		CurrentTheme.Primary,
		CurrentTheme.AI,
		CurrentTheme.Accent,
	}

	logoLines := []string{
		"   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
		"   █                             █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █   ╋       ╋      ╋╋  ╋      █",
		"   █   ╋  ╋╋╋  ╋╋╋╋   ╋ ╋ ╋      █",
		"   █   ╋    ╋  ╋      ╋  ╋╋      █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █                             █",
		"   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
	}

	subtitleStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	hintStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDisabled)

	var sb strings.Builder
	sb.WriteString("\n")

	for i, line := range logoLines {
		colorIdx := i % len(gradient)
		style := lipgloss.NewStyle().Foreground(gradient[colorIdx])
		sb.WriteString(style.Render(line) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("   " + subtitleStyle.Render("AI-powered coding assistant") + "\n")
	sb.WriteString("\n")
	sb.WriteString("   " + hintStyle.Render("Enter to send · Esc to stop · Shift+Tab mode · Ctrl+C exit") + "\n")

	return sb.String()
}

func (m model) renderModeStatus() string {
	var parts []string

	// Mode status
	var icon, label string
	var color lipgloss.Color

	switch m.operationMode {
	case modeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = CurrentTheme.Success
	case modePlan:
		icon = "⏸"
		label = " plan mode on"
		color = CurrentTheme.Warning
	}

	if icon != "" {
		styledIcon := lipgloss.NewStyle().Foreground(color).Render(icon)
		styledLabel := lipgloss.NewStyle().Foreground(color).Render(label)
		hint := lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Render("  shift+tab to toggle")
		parts = append(parts, "  "+styledIcon+styledLabel+hint)
	}

	// Token usage indicator (show when >= 80% of limit)
	tokenUsage := m.renderTokenUsage()
	if tokenUsage != "" {
		parts = append(parts, tokenUsage)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "  ")
}

// renderTokenUsage returns token usage indicator when >= 80% of limit
func (m model) renderTokenUsage() string {
	inputLimit := m.getEffectiveInputLimit()
	if inputLimit == 0 || m.lastInputTokens == 0 {
		return ""
	}

	percent := float64(m.lastInputTokens) / float64(inputLimit) * 100
	if percent < 80 {
		return ""
	}

	// Format: ⚡ 180K/200K (90%)
	used := formatTokenCount(m.lastInputTokens)
	limit := formatTokenCount(inputLimit)

	color := CurrentTheme.Warning // >= 80%
	if percent >= 95 {
		color = CurrentTheme.Error // Red at 95%+
	}

	style := lipgloss.NewStyle().Foreground(color)
	indicator := style.Render(fmt.Sprintf("⚡ %s/%s (%.0f%%)", used, limit, percent))

	// Add /compact hint when >= 90%
	if percent >= 90 {
		hint := lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Render(" (try /compact)")
		indicator += hint
	}

	return indicator
}

func (m model) renderMessages() string {
	if len(m.messages) == 0 {
		return m.renderWelcome()
	}

	// Build a set of message indices to skip (tool results rendered inline with tool calls)
	skipIndices := make(map[int]bool)
	for i, msg := range m.messages {
		if msg.role == "assistant" && len(msg.toolCalls) > 0 {
			// Build set of ToolCall IDs for this assistant message
			toolCallIDs := make(map[string]bool)
			for _, tc := range msg.toolCalls {
				toolCallIDs[tc.ID] = true
			}
			// Mark subsequent tool result messages that match these IDs
			for j := i + 1; j < len(m.messages); j++ {
				nextMsg := m.messages[j]
				if nextMsg.toolResult == nil {
					break
				}
				if toolCallIDs[nextMsg.toolResult.ToolCallID] {
					skipIndices[j] = true
				}
			}
		}
	}

	var sb strings.Builder

	for i, msg := range m.messages {
		// Skip tool results that were rendered inline with their tool calls
		if skipIndices[i] {
			continue
		}

		if msg.toolResult == nil {
			sb.WriteString("\n")
		}

		switch msg.role {
		case "user":
			if msg.toolResult != nil {
				sb.WriteString(m.renderToolResult(msg))
			} else {
				sb.WriteString(m.renderUserMessage(msg))
			}
		case "system":
			sb.WriteString(m.renderSystemMessage(msg))
		case "permission":
			// Skip - rendered separately in View()
		default: // assistant
			sb.WriteString(m.renderAssistantMessage(msg, i, i == len(m.messages)-1))
		}
	}

	sb.WriteString(m.renderPendingToolSpinner())

	return sb.String()
}

func (m model) renderUserMessage(msg chatMessage) string {
	prompt := inputPromptStyle.Render("❯ ")
	content := userMsgStyle.Render(msg.content)
	return prompt + content + "\n"
}

func (m model) renderSystemMessage(msg chatMessage) string {
	return systemMsgStyle.Render(msg.content) + "\n"
}

func (m model) renderToolResult(msg chatMessage) string {
	return m.renderToolResultInline(msg)
}

func (m model) renderAssistantMessage(msg chatMessage, idx int, isLast bool) string {
	var sb strings.Builder
	aiIcon := aiPromptStyle.Render("◆ ")
	aiIndent := "  "

	if msg.content == "" && len(msg.toolCalls) == 0 && m.streaming {
		content := thinkingStyle.Render(m.spinner.View() + " Thinking...")
		sb.WriteString(aiIcon + content + "\n")
	} else if m.streaming && isLast && len(msg.toolCalls) == 0 {
		content := assistantMsgStyle.Render(msg.content + "▌")
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	} else if m.mdRenderer != nil && msg.content != "" {
		rendered, err := m.mdRenderer.Render(msg.content)
		var content string
		if err == nil {
			content = strings.TrimLeft(rendered, " \t\n")
			content = strings.TrimRight(content, " \t\n")
			blankLines := regexp.MustCompile(`\n\s*\n`)
			content = blankLines.ReplaceAllString(content, "\n")
		} else {
			content = msg.content
		}
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	} else if msg.content != "" {
		content := strings.ReplaceAll(msg.content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	}

	if len(msg.toolCalls) > 0 {
		sb.WriteString(m.renderToolCalls(msg, idx))
	}

	return sb.String()
}

func (m model) renderToolCalls(msg chatMessage, msgIdx int) string {
	var sb strings.Builder

	if msg.content != "" {
		sb.WriteString("\n")
	}

	// Build a map from ToolCallID to the corresponding toolResult message
	resultMap := make(map[string]chatMessage)
	for j := msgIdx + 1; j < len(m.messages); j++ {
		nextMsg := m.messages[j]
		if nextMsg.toolResult == nil {
			break
		}
		resultMap[nextMsg.toolResult.ToolCallID] = nextMsg
	}

	for _, tc := range msg.toolCalls {
		if msg.toolCallsExpanded {
			toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s", tc.Name))
			sb.WriteString(toolLine + "\n")
			var params map[string]any
			if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
				for k, v := range params {
					if s, ok := v.(string); ok {
						if len(s) > 80 {
							sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s:", k)) + "\n")
							sb.WriteString(toolResultExpandedStyle.Render(s) + "\n")
						} else {
							sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s: %s", k, s)) + "\n")
						}
					}
				}
			}
		} else {
			// Special formatting for Task tool
			if tc.Name == "Task" {
				toolLine := formatTaskToolCall(tc.Input)
				sb.WriteString(toolCallStyle.Render(toolLine) + "\n")
			} else {
				args := extractToolArgs(tc.Input)
				toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s(%s)", tc.Name, args))
				sb.WriteString(toolLine + "\n")
			}
		}

		// Render the corresponding result inline if found
		if resultMsg, ok := resultMap[tc.ID]; ok {
			sb.WriteString(m.renderToolResultInline(resultMsg))
		}
	}

	return sb.String()
}

// renderToolResultInline renders a tool result inline (without leading newline)
func (m model) renderToolResultInline(msg chatMessage) string {
	toolName := msg.toolName
	if toolName == "" {
		toolName = "Tool"
	}

	// Special handling for Skill tool - show clean summary
	if toolName == "Skill" {
		return m.renderSkillResultInline(msg)
	}

	// Special handling for Task tool - show agent result summary
	if toolName == "Task" {
		return m.renderTaskResultInline(msg)
	}

	// Special handling for TaskOutput - show agent output summary
	if toolName == "TaskOutput" {
		return m.renderTaskOutputResultInline(msg)
	}

	sizeInfo := formatToolResultSize(toolName, msg.toolResult.Content)

	icon := "⎿"
	if msg.toolResult.IsError {
		icon = "✗"
	}

	var sb strings.Builder
	summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s → %s", icon, toolName, sizeInfo))
	sb.WriteString(summary + "\n")

	if msg.expanded || msg.toolResult.IsError {
		lines := strings.Split(msg.toolResult.Content, "\n")
		for _, line := range lines {
			sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// renderSkillResultInline renders a skill result with clean formatting
// Shows: ⎿  Loaded: git:commit [2 scripts, 1 ref]
func (m model) renderSkillResultInline(msg chatMessage) string {
	icon := "⎿"
	if msg.toolResult.IsError {
		icon = "✗"
	}

	var sb strings.Builder

	if msg.toolResult.IsError {
		// For errors, show the error message
		summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, msg.toolResult.Content))
		sb.WriteString(summary + "\n")
		return sb.String()
	}

	// Parse skill info from content
	skillName, scriptCount, refCount := parseSkillResultContent(msg.toolResult.Content)

	// Build resource summary
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

	// Format: Loaded: skill-name [resources]
	result := fmt.Sprintf("Loaded: %s", skillName)
	if len(resources) > 0 {
		result += fmt.Sprintf(" [%s]", strings.Join(resources, ", "))
	}

	summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, result))
	sb.WriteString(summary + "\n")

	// Show expanded content if requested
	if msg.expanded {
		lines := strings.Split(msg.toolResult.Content, "\n")
		for _, line := range lines {
			sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// renderTaskResultInline renders a Task tool result with agent-specific formatting
func (m model) renderTaskResultInline(msg chatMessage) string {
	icon := "⎿"
	if msg.toolResult.IsError {
		icon = "✗"
	}

	var sb strings.Builder
	content := msg.toolResult.Content

	if msg.toolResult.IsError {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  Task → Error", icon)) + "\n")
		sb.WriteString(toolResultExpandedStyle.Render("    "+content) + "\n")
		return sb.String()
	}

	// Parse task info using helper
	agentName := extractField(content, "Agent: ", "Agent")
	taskID := extractField(content, "Task ID: ", "")
	isBackground := strings.Contains(content, "started in background")

	if isBackground && taskID != "" {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  %s → background", icon, agentName)) + "\n")
		sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("     Task ID: %s", taskID)) + "\n")
		sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("     Check:   TaskOutput(\"%s\")", taskID)) + "\n")
	} else {
		turns := extractIntField(content, "Turns: ")
		if turns > 0 {
			sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  %s → Done (%d turns)", icon, agentName, turns)) + "\n")
		} else {
			sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  %s → Done", icon, agentName)) + "\n")
		}
	}

	if msg.expanded {
		for _, line := range strings.Split(content, "\n") {
			sb.WriteString(toolResultExpandedStyle.Render("    "+line) + "\n")
		}
	}

	return sb.String()
}

// extractField extracts a field value from content by prefix, returning defaultVal if not found
func extractField(content, prefix, defaultVal string) string {
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

// extractIntField extracts an integer field value from content by prefix
func extractIntField(content, prefix string) int {
	val := extractField(content, prefix, "")
	if val == "" {
		return 0
	}
	// Parse only leading digits
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

// renderTaskOutputResultInline renders a TaskOutput result with agent-specific formatting
func (m model) renderTaskOutputResultInline(msg chatMessage) string {
	icon := "⎿"
	if msg.toolResult.IsError {
		icon = "✗"
	}

	var sb strings.Builder
	content := msg.toolResult.Content

	if msg.toolResult.IsError {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → Error", icon)) + "\n")
		if content != "" {
			sb.WriteString(toolResultExpandedStyle.Render("    "+content) + "\n")
		}
		return sb.String()
	}

	// Parse task info using extractField helper
	agentName := extractField(content, "Agent: ", "")
	status := extractField(content, "Status: ", "")
	turns := extractIntField(content, "Turns: ")

	// Build summary from parsed fields
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

	sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → %s", icon, summaryText)) + "\n")

	// Show output content if present
	if idx := strings.Index(content, "Output:\n"); idx != -1 {
		outputContent := content[idx+8:]
		outputLines := strings.Split(outputContent, "\n")

		maxLines := 10
		if msg.expanded {
			maxLines = len(outputLines)
		}

		for i, line := range outputLines {
			if i >= maxLines {
				sb.WriteString(toolResultExpandedStyle.Render("    ...") + "\n")
				break
			}
			sb.WriteString(toolResultExpandedStyle.Render("    "+line) + "\n")
		}
	}

	return sb.String()
}

// parseSkillResultContent extracts skill info from skill-invocation content
func parseSkillResultContent(content string) (skillName string, scriptCount, refCount int) {
	skillName = "skill"

	// Extract skill name from <skill-invocation name="...">
	if idx := strings.Index(content, `<skill-invocation name="`); idx != -1 {
		start := idx + len(`<skill-invocation name="`)
		if end := strings.Index(content[start:], `"`); end != -1 {
			skillName = content[start : start+end]
		}
	}

	// Count scripts from "Available scripts" section
	if idx := strings.Index(content, "Available scripts"); idx != -1 {
		section := content[idx:]
		// Count lines starting with "  - " until we hit an empty line or end
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

	// Count refs from "Reference files" section
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

	return
}

func (m model) renderPendingToolSpinner() string {
	interactivePromptActive := m.questionPrompt.IsActive() || (m.planPrompt != nil && m.planPrompt.IsActive())
	if interactivePromptActive {
		return ""
	}

	// Determine which tool is active
	var toolName string
	if m.buildingToolName != "" {
		toolName = m.buildingToolName
	} else if m.pendingToolCalls != nil && m.pendingToolIdx < len(m.pendingToolCalls) {
		toolName = m.pendingToolCalls[m.pendingToolIdx].Name
	} else {
		return ""
	}

	var sb strings.Builder

	// Task tool has special rendering with progress
	if toolName == "Task" {
		status := "Agent starting..."
		if len(m.taskProgress) > 0 {
			status = "Agent running..."
		}
		sb.WriteString(thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), status)) + "\n")
		for _, p := range m.taskProgress {
			sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("     %s", p)) + "\n")
		}
		return sb.String()
	}

	// Standard tool spinner
	sb.WriteString("\n")
	sb.WriteString(toolCallStyle.Render(fmt.Sprintf("⚡%s", toolName)) + "\n")
	sb.WriteString(thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), getToolExecutionDesc(toolName))) + "\n")
	return sb.String()
}

func getToolExecutionDesc(toolName string) string {
	switch toolName {
	case "ExitPlanMode":
		return "Preparing implementation plan..."
	case "Read":
		return "Reading file..."
	case "Write":
		return "Writing file..."
	case "Edit":
		return "Editing file..."
	case "Bash":
		return "Executing command..."
	case "Glob":
		return "Finding files..."
	case "Grep":
		return "Searching files..."
	case "WebFetch":
		return "Fetching web content..."
	case "WebSearch":
		return "Searching the web..."
	case "AskUserQuestion":
		return "Preparing question..."
	case "Skill":
		return "Loading skill..."
	default:
		return "Executing..."
	}
}

// formatTaskToolCall formats a Task tool call with agent type and description
func formatTaskToolCall(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "⚡Task(...)"
	}

	agentType := "Agent"
	if a, ok := params["subagent_type"].(string); ok {
		agentType = a
	}

	desc := ""
	if d, ok := params["description"].(string); ok {
		desc = d
	} else if p, ok := params["prompt"].(string); ok {
		// Use first 40 chars of prompt if no description
		desc = p
		if len(desc) > 40 {
			desc = desc[:40] + "..."
		}
	}

	// Check for background mode
	bgSuffix := ""
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		bgSuffix = " ⏳"
	}

	if desc != "" {
		return fmt.Sprintf("⚡Task(%s: %s)%s", agentType, desc, bgSuffix)
	}
	return fmt.Sprintf("⚡Task(%s)%s", agentType, bgSuffix)
}

func extractToolArgs(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}

	if fp, ok := params["file_path"].(string); ok {
		return fp
	}
	if p, ok := params["pattern"].(string); ok {
		return p
	}
	if p, ok := params["path"].(string); ok {
		return p
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := params[k].(string); ok {
			if len(s) > 60 {
				return s[:60] + "..."
			}
			return s
		}
	}
	return ""
}

func formatToolResultSize(toolName, content string) string {
	switch toolName {
	case "WebFetch":
		size := len(content)
		if size >= 1024*1024 {
			return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
		}
		if size >= 1024 {
			return fmt.Sprintf("%.1f KB", float64(size)/1024)
		}
		return fmt.Sprintf("%d bytes", size)

	case "Write", "Edit":
		start := strings.Index(content, "(")
		if start == -1 {
			return "completed"
		}
		end := strings.Index(content[start:], ")")
		if end == -1 {
			return "completed"
		}
		return content[start+1 : start+end]

	default:
		trimmed := strings.TrimSuffix(content, "\n")
		if trimmed == "" {
			return "0 lines"
		}
		lineCount := strings.Count(trimmed, "\n") + 1
		return fmt.Sprintf("%d lines", lineCount)
	}
}


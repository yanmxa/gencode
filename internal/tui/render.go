// Message rendering: converts chatMessage structs to styled terminal output
// (welcome, user, assistant, tool calls, tool results, summaries, spinners, status bar).
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

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tui/theme"
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
	compactStyle.Heading.Margin = uintPtr(0)
	compactStyle.CodeBlock.Margin = uintPtr(0)
	compactStyle.List.Margin = uintPtr(0)
	compactStyle.Table.Margin = uintPtr(0)

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(compactStyle),
		glamour.WithWordWrap(wrapWidth),
	)
	return renderer
}

func (m model) renderWelcome() string {
	genStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.AI).Bold(true)
	bracketStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
	slashStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent).Bold(true)

	icon := bracketStyle.Render("   < ") +
		genStyle.Render("GEN") +
		slashStyle.Render(" ✦ ") +
		slashStyle.Render("/") +
		bracketStyle.Render(">")

	hintStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDisabled)

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(icon + "\n")
	sb.WriteString("\n")
	sb.WriteString("   " + hintStyle.Render("Enter to send · Esc to stop · Shift+Tab mode · Ctrl+C exit") + "\n")

	return sb.String()
}

func (m model) renderModeStatus() string {
	var parts []string

	if modeStatus := m.renderOperationModeIndicator(); modeStatus != "" {
		parts = append(parts, modeStatus)
	}

	if tokenUsage := m.renderTokenUsage(); tokenUsage != "" {
		parts = append(parts, tokenUsage)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "  ")
}

// renderOperationModeIndicator returns the mode status indicator for auto-accept or plan mode.
func (m model) renderOperationModeIndicator() string {
	var icon, label string
	var color lipgloss.Color

	switch m.operationMode {
	case modeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = theme.CurrentTheme.Success
	case modePlan:
		icon = "⏸"
		label = " plan mode on"
		color = theme.CurrentTheme.Warning
	default:
		return ""
	}

	style := lipgloss.NewStyle().Foreground(color)
	hint := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Render("  shift+tab to toggle")
	return "  " + style.Render(icon+label) + hint
}

// Auto-compact threshold (percentage of context usage)
const autoCompactThreshold = 95

// renderTokenUsage returns token usage indicator.
// Shows context usage with color coding and auto-compact warnings.
func (m model) renderTokenUsage() string {
	inputLimit := m.getEffectiveInputLimit()
	if inputLimit == 0 || m.lastInputTokens == 0 {
		return ""
	}

	percent := float64(m.lastInputTokens) / float64(inputLimit) * 100

	// Only show when >= 50% for awareness
	if percent < 50 {
		return ""
	}

	color, hint := m.tokenUsageColorAndHint(percent)
	style := lipgloss.NewStyle().Foreground(color)

	used := formatTokenCount(m.lastInputTokens)
	limit := formatTokenCount(inputLimit)
	indicator := style.Render(fmt.Sprintf("⚡ %s/%s (%.0f%%)", used, limit, percent))

	if hint != "" {
		indicator += style.Render(hint)
	}

	return indicator
}

// tokenUsageColorAndHint returns the color and hint text for token usage percentage.
func (m model) tokenUsageColorAndHint(percent float64) (lipgloss.Color, string) {
	switch {
	case percent >= autoCompactThreshold:
		return theme.CurrentTheme.Error, " ⚠ auto-compact"
	case percent >= 85:
		return theme.CurrentTheme.Warning, fmt.Sprintf(" (compact at %d%%)", autoCompactThreshold)
	case percent >= 70:
		return theme.CurrentTheme.Accent, ""
	default:
		return theme.CurrentTheme.Muted, ""
	}
}

// buildSkipIndices returns a set of message indices that should be skipped during rendering.
// Tool result messages are skipped when they are rendered inline with their tool calls.
func (m model) buildSkipIndices(startIdx int) map[int]bool {
	skipIndices := make(map[int]bool)
	for i := startIdx; i < len(m.messages); i++ {
		msg := m.messages[i]
		if msg.role != roleAssistant || len(msg.toolCalls) == 0 {
			continue
		}
		// Mark subsequent tool result messages that match these tool calls
		for j := i + 1; j < len(m.messages) && m.messages[j].toolResult != nil; j++ {
			for _, tc := range msg.toolCalls {
				if tc.ID == m.messages[j].toolResult.ToolCallID {
					skipIndices[j] = true
					break
				}
			}
		}
	}
	return skipIndices
}

func (m model) renderMessages() string {
	if len(m.messages) == 0 {
		return m.renderWelcome()
	}
	return m.renderMessageRange(0, len(m.messages), true)
}

// renderPlanForScrollback renders the plan title + markdown content as a styled
// string for pushing into terminal scrollback via tea.Println.
func (m model) renderPlanForScrollback(req *tool.PlanRequest) string {
	if req == nil {
		return ""
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
	sb.WriteString("\n ")
	sb.WriteString(titleStyle.Render("📋 Implementation Plan"))
	sb.WriteString("\n")

	content := req.Plan
	if m.mdRenderer != nil {
		if rendered, err := m.mdRenderer.Render(content); err == nil {
			content = strings.TrimSpace(rendered)
		}
	}
	sb.WriteString(content)

	return sb.String()
}

// renderSingleMessage renders one message at the given index for committing to scrollback.
// It handles the skip logic for inline tool results.
// The trailing newline is trimmed because tea.Println adds its own.
func (m model) renderSingleMessage(idx int) string {
	if idx < 0 || idx >= len(m.messages) {
		return ""
	}

	// Skip tool results that are rendered inline with their tool calls
	if m.messages[idx].toolResult != nil && m.isToolResultInlined(idx) {
		return ""
	}

	return strings.TrimRight(m.renderMessageAt(idx, false), "\n")
}

// renderActiveContent renders all uncommitted messages for the managed region.
// This includes: assistant messages waiting for tool results, partial tool results,
// streaming assistant message, and pending tool spinner.
func (m model) renderActiveContent() string {
	if m.committedCount >= len(m.messages) {
		return m.renderPendingToolSpinner()
	}
	return m.renderMessageRange(m.committedCount, len(m.messages), true)
}

// isToolResultInlined checks if the tool result at idx was rendered inline with its tool call.
func (m model) isToolResultInlined(idx int) bool {
	msg := m.messages[idx]
	if msg.toolResult == nil {
		return false
	}
	toolCallID := msg.toolResult.ToolCallID

	// Look backwards for the assistant message that has the matching tool call
	for j := idx - 1; j >= 0; j-- {
		prev := m.messages[j]
		if prev.role == roleAssistant && len(prev.toolCalls) > 0 {
			for _, tc := range prev.toolCalls {
				if tc.ID == toolCallID {
					return true
				}
			}
			// Found an assistant message with tool calls but no match - stop searching
			break
		}
		// Skip over other tool result messages in the sequence
		if prev.toolResult != nil {
			continue
		}
		// Non-tool-result, non-assistant message breaks the chain
		break
	}
	return false
}

// renderMessageAt renders a single message at the given index.
func (m model) renderMessageAt(idx int, isStreaming bool) string {
	msg := m.messages[idx]
	var sb strings.Builder

	if msg.toolResult == nil {
		sb.WriteString("\n")
	}

	switch msg.role {
	case roleUser:
		if msg.toolResult != nil {
			sb.WriteString(m.renderToolResult(msg))
		} else {
			sb.WriteString(m.renderUserMessage(msg))
		}
	case roleNotice:
		sb.WriteString(m.renderSystemMessage(msg))
	case roleAssistant:
		sb.WriteString(m.renderAssistantMessage(msg, idx, isStreaming))
	}

	return sb.String()
}

// renderMessageRange renders messages from startIdx to endIdx with skip logic and spinner.
func (m model) renderMessageRange(startIdx, endIdx int, includeSpinner bool) string {
	skipIndices := m.buildSkipIndices(startIdx)
	var sb strings.Builder

	lastIdx := endIdx - 1
	isLastStreaming := m.stream.active && lastIdx >= 0 && m.messages[lastIdx].role == roleAssistant

	for i := startIdx; i < endIdx; i++ {
		if skipIndices[i] {
			continue
		}
		isStreaming := i == lastIdx && isLastStreaming
		sb.WriteString(m.renderMessageAt(i, isStreaming))
	}

	if includeSpinner {
		sb.WriteString(m.renderPendingToolSpinner())
	}

	return sb.String()
}

func (m model) renderUserMessage(msg chatMessage) string {
	// Handle compact summary messages with collapse/expand
	if msg.isSummary {
		return m.renderSummaryMessage(msg)
	}

	var sb strings.Builder
	prompt := inputPromptStyle.Render("❯ ")

	// Render image indicators in Claude Code style
	if len(msg.images) > 0 {
		var parts []string
		for i := range msg.images {
			parts = append(parts, pendingImageStyle.Render(fmt.Sprintf("[Image #%d]", i+1)))
		}
		sb.WriteString(prompt + strings.Join(parts, " ") + "\n")
	}

	// Render text content
	if msg.content != "" {
		sb.WriteString(prompt + userMsgStyle.Render(msg.content) + "\n")
	}

	return sb.String()
}

// renderPendingImages renders indicator for clipboard images waiting to be sent
// Uses Claude Code style with selection mode
func (m model) renderPendingImages() string {
	if len(m.pendingImages) == 0 {
		return ""
	}

	var parts []string
	for i := range m.pendingImages {
		label := fmt.Sprintf("[Image #%d]", i+1)
		if m.imageSelectMode && i == m.selectedImageIdx {
			// Highlight selected image
			parts = append(parts, selectedImageStyle.Render(label))
		} else {
			parts = append(parts, pendingImageStyle.Render(label))
		}
	}

	var hint string
	if m.imageSelectMode {
		hint = pendingImageHintStyle.Render(" ← prev · → next · Del remove · Esc cancel")
	} else {
		hint = pendingImageHintStyle.Render(" (↑ to select)")
	}

	return "  " + strings.Join(parts, " ") + hint + "\n"
}

// renderSummaryMessage renders a compact summary with collapse/expand support
func (m model) renderSummaryMessage(msg chatMessage) string {
	var sb strings.Builder

	icon := aiPromptStyle.Render("● ")
	title := fmt.Sprintf("Compacted %d messages → 1 summary", msg.summaryCount)
	header := icon + toolCallStyle.Render(title)

	hintText := " (Ctrl+O to expand)"
	if msg.expanded {
		hintText = " (Ctrl+O to collapse)"
	}
	sb.WriteString(header + toolResultStyle.Render(hintText) + "\n")

	if msg.expanded {
		content := msg.content
		if m.mdRenderer != nil {
			if rendered, err := m.mdRenderer.Render(msg.content); err == nil {
				content = strings.TrimSpace(rendered)
			}
		}
		sb.WriteString(toolResultExpandedStyle.Render(content) + "\n")
	}

	return sb.String()
}

func (m model) renderSystemMessage(msg chatMessage) string {
	return systemMsgStyle.Render(msg.content) + "\n"
}

func (m model) renderToolResult(msg chatMessage) string {
	return m.renderToolResultInline(msg)
}

func (m model) renderAssistantMessage(msg chatMessage, idx int, isLast bool) string {
	var sb strings.Builder
	aiIcon := aiPromptStyle.Render("● ")
	aiIndent := "  "

	// Display thinking content (reasoning_content) if available
	if msg.thinking != "" {
		thinkingIcon := thinkingContentStyle.Render("✦ ")
		thinkingContent := thinkingContentStyle.Render(msg.thinking)
		thinkingContent = strings.ReplaceAll(thinkingContent, "\n", "\n"+aiIndent)
		sb.WriteString(thinkingIcon + thinkingContent + "\n\n")
	}

	// Render content based on streaming state
	content := m.formatAssistantContent(msg, isLast)
	if content != "" {
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	}

	if len(msg.toolCalls) > 0 {
		sb.WriteString(m.renderToolCalls(msg, idx))
	}

	return sb.String()
}

// formatAssistantContent formats the assistant message content based on streaming state.
func (m model) formatAssistantContent(msg chatMessage, isLast bool) string {
	// Waiting for response with no content yet
	if msg.content == "" && len(msg.toolCalls) == 0 && m.stream.active && msg.thinking == "" {
		return thinkingStyle.Render(m.spinner.View() + " Thinking...")
	}

	// Streaming in progress - show cursor
	if m.stream.active && isLast && len(msg.toolCalls) == 0 {
		return assistantMsgStyle.Render(msg.content + "▌")
	}

	// No content to render
	if msg.content == "" {
		return ""
	}

	// Render markdown if available
	if m.mdRenderer != nil {
		return m.renderMarkdownContent(msg.content)
	}

	return msg.content
}

// renderMarkdownContent renders content through the markdown renderer.
func (m model) renderMarkdownContent(content string) string {
	rendered, err := m.mdRenderer.Render(content)
	if err != nil {
		return content
	}

	result := strings.TrimLeft(rendered, " \t\n")
	result = strings.TrimRight(result, " \t\n")

	// Glamour renders "blank" lines filled with ANSI escape codes (e.g. colored spaces).
	// Match lines containing only ANSI codes and/or whitespace, then collapse.
	blankLines := regexp.MustCompile(`\n((?:\x1b\[[0-9;]*[a-zA-Z]|[ \t])*\n)+`)
	return blankLines.ReplaceAllString(result, "\n")
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
			toolLine := toolCallStyle.Render(fmt.Sprintf("⚙ %s", tc.Name))
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
				toolLine := toolCallStyle.Render(fmt.Sprintf("⚙ %s(%s)", tc.Name, args))
				sb.WriteString(toolLine + "\n")
			}
		}

		// Render the corresponding result inline if found
		if resultMsg, ok := resultMap[tc.ID]; ok {
			sb.WriteString(m.renderToolResultInline(resultMsg))
		} else if m.toolExec.parallel && tc.Name == "Task" {
			// Parallel mode: show live progress inline under each Task
			sb.WriteString(m.renderTaskProgressInline(tc))
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
		for line := range strings.SplitSeq(msg.toolResult.Content, "\n") {
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
		for line := range strings.SplitSeq(msg.toolResult.Content, "\n") {
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
		// Build stats summary: (N tool uses · XYk tokens · Nm Ns)
		toolUses := extractIntField(content, "ToolUses: ")
		tokens := extractIntField(content, "Tokens: ")
		duration := extractField(content, "Duration: ", "")

		var stats []string
		if toolUses > 0 {
			stats = append(stats, fmt.Sprintf("%d tool uses", toolUses))
		}
		if tokens > 0 {
			stats = append(stats, formatTokenCount(tokens)+" tokens")
		}
		if duration != "" {
			stats = append(stats, duration)
		}

		agentLine := fmt.Sprintf("  %s  %s → Done", icon, agentName)
		if len(stats) > 0 {
			agentLine += " (" + strings.Join(stats, " · ") + ")"
		}
		sb.WriteString(toolResultStyle.Render(agentLine) + "\n")
	}

	if msg.expanded {
		for line := range strings.SplitSeq(content, "\n") {
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
	// Find end of leading digits
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
	if _, outputContent, found := strings.Cut(content, "Output:\n"); found {
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

	// Parallel mode with Task tools: progress rendered inline by renderToolCalls
	if m.toolExec.parallel && m.hasParallelTaskTools() {
		return ""
	}

	// Determine which tool is active
	var toolName string
	if m.stream.buildingTool != "" {
		toolName = m.stream.buildingTool
	} else if m.toolExec.pendingCalls != nil && m.toolExec.currentIdx < len(m.toolExec.pendingCalls) {
		toolName = m.toolExec.pendingCalls[m.toolExec.currentIdx].Name
	} else {
		return ""
	}

	var sb strings.Builder

	// Task tool has special rendering with per-agent progress
	if toolName == "Task" {
		progress := m.taskProgress[m.toolExec.currentIdx]
		status := "Agent starting..."
		if len(progress) > 0 {
			status = "Agent running..."
		}
		sb.WriteString(thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), status)) + "\n")
		for _, p := range progress {
			sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("     %s", p)) + "\n")
		}
		return sb.String()
	}

	// Standard tool spinner (tool name already rendered by renderToolCalls)
	sb.WriteString(thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), getToolExecutionDesc(toolName))) + "\n")
	return sb.String()
}

// hasParallelTaskTools returns true if any pending tool call is a Task tool
func (m model) hasParallelTaskTools() bool {
	for _, tc := range m.toolExec.pendingCalls {
		if tc.Name == "Task" {
			return true
		}
	}
	return false
}

// renderTaskProgressInline renders live progress for a parallel Task tool call
// directly under its ⚙ Task(...) line. Shows spinner+progress while running,
// or ✓ Done when completed (before results are committed to messages).
func (m model) renderTaskProgressInline(tc message.ToolCall) string {
	idx, ok := m.findPendingToolIndex(tc.ID)
	if !ok {
		return ""
	}

	var sb strings.Builder

	// Check if completed in parallel results (not yet committed to messages)
	if _, done := m.toolExec.parallelResults[idx]; done {
		sb.WriteString(toolResultStyle.Render("  ✓ Done") + "\n")
		return sb.String()
	}

	// Show spinner and progress lines
	progress := m.taskProgress[idx]
	status := "starting..."
	if len(progress) > 0 {
		status = "running..."
	}
	sb.WriteString(thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), status)) + "\n")
	for _, p := range progress {
		sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("     %s", p)) + "\n")
	}
	return sb.String()
}

// findPendingToolIndex finds a tool call's index in pendingToolCalls by ID
func (m model) findPendingToolIndex(toolCallID string) (int, bool) {
	for i, tc := range m.toolExec.pendingCalls {
		if tc.ID == toolCallID {
			return i, true
		}
	}
	return -1, false
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
		return "⚙ Task(...)"
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
		return fmt.Sprintf("⚙ Task(%s: %s)%s", agentType, desc, bgSuffix)
	}
	return fmt.Sprintf("⚙ Task(%s)%s", agentType, bgSuffix)
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

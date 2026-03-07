// Pure message rendering functions that take explicit parameters instead of model state.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

const (
	// MinWrapWidth is the minimum markdown wrap width.
	MinWrapWidth = 40

	// AutoCompactThreshold is the percentage of context usage that triggers auto-compact.
	AutoCompactThreshold = 95
)

// RenderWelcome renders the welcome screen.
func RenderWelcome() string {
	genStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.AI).Bold(true)
	bracketStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
	slashStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent).Bold(true)

	icon := bracketStyle.Render("   < ") +
		genStyle.Render("GEN") +
		slashStyle.Render(" ✦ ") +
		slashStyle.Render("/") +
		bracketStyle.Render(">")

	return "\n" + icon
}

// OperationModeParams holds the parameters needed for rendering mode status.
type OperationModeParams struct {
	Mode        int // 0=normal, 1=autoAccept, 2=plan
	InputTokens int
	InputLimit  int
	ModelName   string // Current model name shown right-aligned
	Width       int    // Terminal width for right-alignment
}

// RenderModeStatus renders the combined mode status line.
func RenderModeStatus(params OperationModeParams) string {
	var parts []string

	if modeStatus := RenderOperationModeIndicator(params.Mode); modeStatus != "" {
		parts = append(parts, modeStatus)
	}

	if tokenUsage := RenderTokenUsage(params.InputTokens, params.InputLimit); tokenUsage != "" {
		parts = append(parts, tokenUsage)
	}

	left := strings.Join(parts, "  ")

	if params.ModelName == "" || params.Width <= 0 {
		return left
	}

	// Render model name right-aligned with muted style
	modelStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	right := modelStyle.Render(params.ModelName)

	gap := max(2, params.Width-lipgloss.Width(left)-lipgloss.Width(right)-1)

	return left + strings.Repeat(" ", gap) + right
}

// RenderOperationModeIndicator returns the mode status indicator for auto-accept or plan mode.
func RenderOperationModeIndicator(mode int) string {
	var icon, label string
	var color lipgloss.Color

	switch appmode.OperationMode(mode) {
	case appmode.AutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = theme.CurrentTheme.Success
	case appmode.Plan:
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

// RenderTokenUsage returns token usage indicator.
// Shows context usage with color coding and auto-compact warnings.
func RenderTokenUsage(inputTokens, inputLimit int) string {
	if inputLimit == 0 || inputTokens == 0 {
		return ""
	}

	percent := float64(inputTokens) / float64(inputLimit) * 100

	// Only show when >= 50% for awareness
	if percent < 50 {
		return ""
	}

	color, hint := TokenUsageColorAndHint(percent)
	style := lipgloss.NewStyle().Foreground(color)

	used := FormatTokenCount(inputTokens)
	limit := FormatTokenCount(inputLimit)
	indicator := style.Render(fmt.Sprintf("⚡ %s/%s (%.0f%%)", used, limit, percent))

	if hint != "" {
		indicator += style.Render(hint)
	}

	return indicator
}

// toolResultIcon returns the icon for tool results based on error state.
func toolResultIcon(isError bool) string {
	if isError {
		return "✗"
	}
	return "⎿"
}

// TokenUsageColorAndHint returns the color and hint text for token usage percentage.
func TokenUsageColorAndHint(percent float64) (lipgloss.Color, string) {
	switch {
	case percent >= AutoCompactThreshold:
		return theme.CurrentTheme.Error, " ⚠ auto-compact"
	case percent >= 85:
		return theme.CurrentTheme.Warning, fmt.Sprintf(" (compact at %d%%)", AutoCompactThreshold)
	case percent >= 70:
		return theme.CurrentTheme.Accent, ""
	default:
		return theme.CurrentTheme.Muted, ""
	}
}

// RenderUserMessage renders a user message with prompt and optional images.
func RenderUserMessage(content string, images []message.ImageData, mdRenderer *MDRenderer) string {
	var sb strings.Builder
	prompt := InputPromptStyle.Render("❯ ")

	// Render image indicators
	if len(images) > 0 {
		var parts []string
		for i := range images {
			parts = append(parts, PendingImageStyle.Render(fmt.Sprintf("[Image #%d]", i+1)))
		}
		sb.WriteString(prompt + strings.Join(parts, " ") + "\n")
	}

	// Render text content
	if content != "" {
		sb.WriteString(prompt + UserMsgStyle.Render(content) + "\n")
	}

	return sb.String()
}

// PendingImagesParams holds the parameters for rendering pending images.
type PendingImagesParams struct {
	Pending     []message.ImageData
	SelectMode  bool
	SelectedIdx int
}

// RenderPendingImages renders indicator for clipboard images waiting to be sent.
func RenderPendingImages(params PendingImagesParams) string {
	if len(params.Pending) == 0 {
		return ""
	}

	var parts []string
	for i := range params.Pending {
		label := fmt.Sprintf("[Image #%d]", i+1)
		if params.SelectMode && i == params.SelectedIdx {
			parts = append(parts, SelectedImageStyle.Render(label))
		} else {
			parts = append(parts, PendingImageStyle.Render(label))
		}
	}

	var hint string
	if params.SelectMode {
		hint = PendingImageHintStyle.Render(" ← prev · → next · Del remove · Esc cancel")
	} else {
		hint = PendingImageHintStyle.Render(" (↑ to select)")
	}

	return "  " + strings.Join(parts, " ") + hint + "\n"
}

// RenderSystemMessage renders a system/notice message.
func RenderSystemMessage(content string) string {
	return SystemMsgStyle.Render(content) + "\n"
}

// AssistantParams holds the parameters for rendering an assistant message.
type AssistantParams struct {
	Content           string
	Thinking          string
	ToolCalls         []message.ToolCall
	ToolCallsExpanded bool
	StreamActive      bool
	IsLast            bool
	SpinnerView       string
	MDRenderer        *MDRenderer
}

// RenderAssistantMessage renders an assistant message with thinking, content, and tool calls.
func RenderAssistantMessage(params AssistantParams) string {
	var sb strings.Builder
	aiIcon := AIPromptStyle.Render("● ")
	aiIndent := "  "

	// Display thinking content (reasoning_content) if available
	if params.Thinking != "" {
		thinkingIcon := ThinkingContentStyle.Render("✦ ")
		thinkingContent := ThinkingContentStyle.Render(params.Thinking)
		thinkingContent = strings.ReplaceAll(thinkingContent, "\n", "\n"+aiIndent)
		sb.WriteString(thinkingIcon + thinkingContent + "\n\n")
	}

	// Render content based on streaming state
	content := FormatAssistantContent(params)
	if content != "" {
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	}

	return sb.String()
}

// FormatAssistantContent formats the assistant message content based on streaming state.
func FormatAssistantContent(params AssistantParams) string {
	// Waiting for response with no content yet
	if params.Content == "" && len(params.ToolCalls) == 0 && params.StreamActive && params.Thinking == "" {
		return ThinkingStyle.Render(params.SpinnerView + " Thinking...")
	}

	// Streaming in progress - show cursor
	if params.StreamActive && params.IsLast && len(params.ToolCalls) == 0 {
		return AssistantMsgStyle.Render(params.Content + "▌")
	}

	// No content to render
	if params.Content == "" {
		return ""
	}

	// Render markdown if available
	if params.MDRenderer != nil {
		return RenderMarkdownContent(params.MDRenderer, params.Content)
	}

	return params.Content
}

// RenderMarkdownContent renders content through the markdown renderer.
func RenderMarkdownContent(mdRenderer *MDRenderer, content string) string {
	rendered, err := mdRenderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}

// ToolCallsParams holds the parameters for rendering tool calls.
type ToolCallsParams struct {
	ToolCalls         []message.ToolCall
	ToolCallsExpanded bool
	// ResultMap maps ToolCallID to the corresponding tool result message data.
	ResultMap map[string]ToolResultData
	// ParallelMode indicates whether tools are executing in parallel.
	ParallelMode bool
	// ParallelResults tracks completed parallel results by index.
	ParallelResults map[int]bool
	// TaskProgress tracks agent progress messages by index.
	TaskProgress map[int][]string
	// PendingCalls for finding tool index.
	PendingCalls []message.ToolCall
	// SpinnerView is the current spinner frame.
	SpinnerView string
}

// ToolResultData holds the data needed to render a tool result inline.
type ToolResultData struct {
	ToolName string
	Content  string
	IsError  bool
	Expanded bool
}

// RenderToolCalls renders the tool calls section of an assistant message.
func RenderToolCalls(params ToolCallsParams) string {
	var sb strings.Builder

	for _, tc := range params.ToolCalls {
		if params.ToolCallsExpanded {
			toolLine := ToolCallStyle.Render(fmt.Sprintf("⚙ %s", tc.Name))
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
			// Special formatting for Task tool
			if tc.Name == "Task" {
				toolLine := FormatTaskToolCall(tc.Input)
				sb.WriteString(ToolCallStyle.Render(toolLine) + "\n")
			} else {
				args := ExtractToolArgs(tc.Input)
				toolLine := ToolCallStyle.Render(fmt.Sprintf("⚙ %s(%s)", tc.Name, args))
				sb.WriteString(toolLine + "\n")
			}
		}

		// Render the corresponding result inline if found
		if resultData, ok := params.ResultMap[tc.ID]; ok {
			sb.WriteString(RenderToolResultInline(resultData, nil))
		} else if params.ParallelMode && tc.Name == "Task" {
			// Parallel mode: show live progress inline under each Task
			sb.WriteString(RenderTaskProgressInline(tc, params.PendingCalls, params.ParallelResults, params.TaskProgress, params.SpinnerView))
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

	// Special handling for Skill tool
	if toolName == "Skill" {
		return RenderSkillResultInline(data)
	}

	// Special handling for Task tool
	if toolName == "Task" {
		return RenderTaskResultInline(data)
	}

	// Special handling for TaskOutput
	if toolName == "TaskOutput" {
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

	// Parse skill info from content
	skillName, scriptCount, refCount := ParseSkillResultContent(data.Content)

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

	summary := ToolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, result))
	sb.WriteString(summary + "\n")

	// Show expanded content if requested
	if data.Expanded {
		for line := range strings.SplitSeq(data.Content, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// RenderTaskResultInline renders a Task tool result with agent-specific formatting.
func RenderTaskResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content

	if data.IsError {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  Task → Error", icon)) + "\n")
		sb.WriteString(ToolResultExpandedStyle.Render("    "+content) + "\n")
		return sb.String()
	}

	// Parse task info using helper
	agentName := ExtractField(content, "Agent: ", "Agent")
	taskID := ExtractField(content, "Task ID: ", "")
	isBackground := strings.Contains(content, "started in background")

	if isBackground && taskID != "" {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  %s  %s → background", icon, agentName)) + "\n")
		sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("     Task ID: %s", taskID)) + "\n")
		sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("     Check:   TaskOutput(\"%s\")", taskID)) + "\n")
	} else {
		// Build stats summary: (N tool uses, XYk tokens, Nm Ns)
		toolUses := ExtractIntField(content, "ToolUses: ")
		tokens := ExtractIntField(content, "Tokens: ")
		duration := ExtractField(content, "Duration: ", "")

		var stats []string
		if toolUses > 0 {
			stats = append(stats, fmt.Sprintf("%d tool uses", toolUses))
		}
		if tokens > 0 {
			stats = append(stats, FormatTokenCount(tokens)+" tokens")
		}
		if duration != "" {
			stats = append(stats, duration)
		}

		agentLine := fmt.Sprintf("  %s  %s → Done", icon, agentName)
		if len(stats) > 0 {
			agentLine += " (" + strings.Join(stats, " · ") + ")"
		}
		sb.WriteString(ToolResultStyle.Render(agentLine) + "\n")
	}

	if data.Expanded {
		for line := range strings.SplitSeq(content, "\n") {
			sb.WriteString(ToolResultExpandedStyle.Render("    "+line) + "\n")
		}
	}

	return sb.String()
}

// RenderTaskOutputResultInline renders a TaskOutput result with agent-specific formatting.
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

	// Parse task info
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

	// Show output content if present
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
			sb.WriteString(ToolResultExpandedStyle.Render("    "+line) + "\n")
		}
	}

	return sb.String()
}

// RenderPlanForScrollback renders the plan title + markdown content as a styled string.
func RenderPlanForScrollback(plan string, mdRenderer *MDRenderer) string {
	if plan == "" {
		return ""
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
	sb.WriteString("\n ")
	sb.WriteString(titleStyle.Render("📋 Implementation Plan"))
	sb.WriteString("\n")

	content := plan
	if mdRenderer != nil {
		if rendered, err := mdRenderer.Render(content); err == nil {
			content = strings.TrimSpace(rendered)
		}
	}
	sb.WriteString(content)

	return sb.String()
}

// ParseSkillResultContent extracts skill info from skill-invocation content.
func ParseSkillResultContent(content string) (skillName string, scriptCount, refCount int) {
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

// GetToolExecutionDesc returns a human-readable description for a tool being executed.
func GetToolExecutionDesc(toolName string) string {
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

// FormatTaskToolCall formats a Task tool call with agent type and description.
func FormatTaskToolCall(input string) string {
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
		desc = p
		if len(desc) > 40 {
			desc = desc[:40] + "..."
		}
	}

	bgSuffix := ""
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		bgSuffix = " ⏳"
	}

	if desc != "" {
		return fmt.Sprintf("⚙ Task(%s: %s)%s", agentType, desc, bgSuffix)
	}
	return fmt.Sprintf("⚙ Task(%s)%s", agentType, bgSuffix)
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

// FormatToolResultSize returns a human-readable size description for a tool result.
func FormatToolResultSize(toolName, content string) string {
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

// FormatTokenCount formats a token count for display.
func FormatTokenCount(count int) string {
	if count >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	if count >= 1000 {
		return fmt.Sprintf("%dK", count/1000)
	}
	return fmt.Sprintf("%d", count)
}

// Pure message rendering functions that take explicit parameters instead of model state.
package output

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// OperationMode mirrors OperationMode to avoid importing setting in the render layer.
type OperationMode int

const (
	ModeNormal OperationMode = iota
	ModeAutoAccept
	ModePlan
	ModeBypassPermissions
)

const (
	// minWrapWidth is the minimum markdown wrap width.
	minWrapWidth = 40

	// autoCompactThreshold is the percentage of context usage that triggers auto-compact.
	autoCompactThreshold = 95

	// agentContentIndent is the extra indent for agent prompt/response content
	// beyond toolResultExpandedStyle's PaddingLeft(4). Total indent = 4 + 4 = 8 chars.
	agentContentIndent = "    "
)

// RenderWelcome renders the welcome screen.
func RenderWelcome() string {
	genStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.AI).Bold(true)
	bracketStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
	slashStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Accent).Bold(true)

	icon := bracketStyle.Render("   < ") +
		genStyle.Render("GEN") +
		slashStyle.Render(" ✦ ") +
		slashStyle.Render("/") +
		bracketStyle.Render(">")

	return "\n" + icon
}

// OperationModeParams holds the parameters needed for rendering mode status.
type OperationModeParams struct {
	Mode          OperationMode
	InputTokens   int
	InputLimit    int
	ModelName     string
	Width         int
	ThinkingLevel llm.ThinkingLevel
	QueueCount    int
}

// RenderModeStatus renders the combined mode status line.
func RenderModeStatus(params OperationModeParams) string {
	parts := make([]string, 0, 4)

	if modeStatus := RenderOperationModeIndicator(params.Mode); modeStatus != "" {
		parts = append(parts, modeStatus)
	}

	if thinkingStatus := RenderThinkingIndicator(params.ThinkingLevel); thinkingStatus != "" {
		parts = append(parts, thinkingStatus)
	}

	if tokenUsage := renderTokenUsage(params.InputTokens, params.InputLimit); tokenUsage != "" {
		parts = append(parts, tokenUsage)
	}

	if queueBadge := renderQueueBadge(params.QueueCount); queueBadge != "" {
		parts = append(parts, queueBadge)
	}

	left := strings.Join(parts, "  ")

	if params.ModelName == "" || params.Width <= 0 {
		return left
	}

	modelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	right := modelStyle.Render(params.ModelName)
	gap := max(2, params.Width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	return left + strings.Repeat(" ", gap) + right
}

// RenderOperationModeIndicator returns the mode status indicator for auto-accept or plan mode.
func RenderOperationModeIndicator(mode OperationMode) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch mode {
	case ModeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = kit.CurrentTheme.Success
	case ModePlan:
		icon = "⏸"
		label = " plan mode on"
		color = kit.CurrentTheme.Warning
	case ModeBypassPermissions:
		icon = "⏩"
		label = " bypass permissions on"
		color = kit.CurrentTheme.Error
	default:
		return ""
	}

	style := lipgloss.NewStyle().Foreground(color)
	hint := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Render("  shift+tab to toggle")
	return "  " + style.Render(icon+label) + hint
}

// RenderThinkingIndicator returns a styled indicator for the current thinking level.
func RenderThinkingIndicator(level llm.ThinkingLevel) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch level {
	case llm.ThinkingNormal:
		icon = "✦"
		label = " think"
		color = kit.CurrentTheme.Accent
	case llm.ThinkingHigh:
		icon = "✦✦"
		label = " think+"
		color = kit.CurrentTheme.Primary
	case llm.ThinkingUltra:
		icon = "✦✦✦"
		label = " ultrathink"
		color = kit.CurrentTheme.AI
	default:
		return ""
	}

	style := lipgloss.NewStyle().Foreground(color)
	return "  " + style.Render(icon+label)
}

// renderTokenUsage returns token usage indicator.
func renderTokenUsage(inputTokens, inputLimit int) string {
	if inputLimit == 0 || inputTokens == 0 {
		return ""
	}

	percent := float64(inputTokens) / float64(inputLimit) * 100
	if percent < 50 {
		return ""
	}

	color, hint := tokenUsageColorAndHint(percent)
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

// tokenUsageColorAndHint returns the color and hint text for token usage percentage.
func tokenUsageColorAndHint(percent float64) (lipgloss.TerminalColor, string) {
	if percent >= autoCompactThreshold {
		return kit.CurrentTheme.Error, " ⚠ auto-compact"
	}
	if percent >= 85 {
		return kit.CurrentTheme.Warning, fmt.Sprintf(" (compact at %d%%)", autoCompactThreshold)
	}
	if percent >= 70 {
		return kit.CurrentTheme.Accent, ""
	}
	return kit.CurrentTheme.Muted, ""
}

// RenderTokenWarning returns a warning line when context usage is high.
// Displayed above the input separator to alert the user.
func RenderTokenWarning(inputTokens, inputLimit int, compactSuppressed bool) string {
	if inputLimit == 0 || inputTokens == 0 || compactSuppressed {
		return ""
	}

	percent := float64(inputTokens) / float64(inputLimit) * 100
	if percent < 80 {
		return ""
	}

	untilCompact := max(int(autoCompactThreshold-percent), 0)

	if percent >= autoCompactThreshold {
		style := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
		return "  " + style.Render(fmt.Sprintf("⚠ Context nearly full (%d%% used) — auto-compact imminent", int(percent)))
	}
	if percent >= 85 {
		style := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning)
		return "  " + style.Render(fmt.Sprintf("⚡ %d%% until auto-compact", untilCompact))
	}
	style := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	return "  " + style.Render(fmt.Sprintf("⚡ %d%% until auto-compact", untilCompact))
}

// RenderPlanForScrollback renders the plan markdown content for scrollback.
func RenderPlanForScrollback(plan string, mdRenderer *MDRenderer) string {
	if plan == "" {
		return ""
	}

	content := plan
	if mdRenderer != nil {
		if rendered, err := mdRenderer.Render(content); err == nil {
			content = strings.TrimSpace(rendered)
		}
	}
	return content
}

// FormatTokenCount formats a token count for display.
func FormatTokenCount(count int) string {
	switch {
	case count >= 1000000:
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	case count >= 1000:
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	default:
		return fmt.Sprintf("%d", count)
	}
}

var (
	userMsgStyle      = lipgloss.NewStyle()
	assistantMsgStyle = lipgloss.NewStyle()

	InputPromptStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Primary).
				Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.AI).
			Bold(true)

	SeparatorStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Separator)

	ThinkingStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Muted)

	systemMsgStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.TextDim).
			PaddingLeft(2)

	toolCallStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Text)

	toolResultStyle = toolCallStyle

	toolResultExpandedStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.TextDim).
				PaddingLeft(4)

	agentLabelStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Success)

	trackerPendingStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Muted)

	trackerInProgressStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Primary).
				Bold(true)

	trackerCompletedStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.TextDisabled).
				Strikethrough(true)

	PendingImageStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Primary)

	pendingImageHintStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Muted)

	SelectedImageStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.TextBright).
				Background(kit.CurrentTheme.Primary).
				Bold(true)
)
var inlineImageTokenPattern = regexp.MustCompile(`\[Image #\d+\]`)

// RenderUserMessage renders a user message with prompt and optional images.
func RenderUserMessage(content, displayContent string, images []core.Image, mdRenderer *MDRenderer, width int) string {
	var sb strings.Builder
	prompt := InputPromptStyle.Render("❯ ")
	if displayContent == "" {
		displayContent = content
	}

	if len(images) > 0 && inlineImageTokenPattern.MatchString(displayContent) {
		sb.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top,
			prompt,
			userMsgStyle.Render(styleInlineImageTokens(displayContent)),
		) + "\n")
		return sb.String()
	}

	if len(images) > 0 {
		imgParts := make([]string, 0, len(images))
		for i := range images {
			imgParts = append(imgParts, PendingImageStyle.Render(fmt.Sprintf("[Image #%d]", i+1)))
		}
		imageLabel := strings.Join(imgParts, " ")
		if content != "" {
			sb.WriteString(prompt + imageLabel + " " + userMsgStyle.Render(content) + "\n")
		} else {
			sb.WriteString(prompt + imageLabel + "\n")
		}
	} else if content != "" {
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, prompt, userMsgStyle.Render(content)) + "\n")
	}

	return sb.String()
}

func styleInlineImageTokens(content string) string {
	return inlineImageTokenPattern.ReplaceAllStringFunc(content, func(token string) string {
		return PendingImageStyle.Render(token)
	})
}

// AssistantParams holds the parameters for rendering an assistant core.
type AssistantParams struct {
	Content           string
	Thinking          string
	ToolCalls         []core.ToolCall
	ToolCallsExpanded bool
	StreamActive      bool
	IsLast            bool
	SpinnerView       string
	MDRenderer        *MDRenderer
	Width             int
	ExecutingTool     string
}

// RenderAssistantMessage renders an assistant message with thinking, content, and tool calls.
func RenderAssistantMessage(params AssistantParams) string {
	var sb strings.Builder
	aiIcon := aiPromptStyle.Render("● ")
	if params.StreamActive && params.IsLast {
		aiIcon = aiPromptStyle.Render(params.SpinnerView + " ")
	}

	if params.Thinking != "" {
		wrapWidth := max(params.Width-2, minWrapWidth)
		wrapped := lipgloss.NewStyle().Width(wrapWidth).Render(params.Thinking)
		var lines []string
		for _, line := range strings.Split(wrapped, "\n") {
			if strings.TrimSpace(line) != "" {
				if lines == nil {
					lines = make([]string, 0, 8)
				}
				lines = append(lines, ThinkingStyle.Render(line))
			}
		}
		thinkingIcon := ThinkingStyle.Render("✦ ")
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, thinkingIcon, strings.Join(lines, "\n")) + "\n\n")
	}

	content := formatAssistantContent(params)
	if content != "" {
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, aiIcon, content) + "\n")
	}

	return sb.String()
}

// formatAssistantContent formats the assistant message content based on streaming state.
func formatAssistantContent(params AssistantParams) string {
	if params.Content == "" && len(params.ToolCalls) == 0 && params.StreamActive && params.Thinking == "" {
		if params.ExecutingTool != "" {
			return ThinkingStyle.Render(getToolExecutionDesc(params.ExecutingTool))
		}
		return ThinkingStyle.Render("Thinking...")
	}

	if params.StreamActive && params.IsLast && len(params.ToolCalls) == 0 {
		return assistantMsgStyle.Render(params.Content + "▌")
	}

	if params.Content == "" {
		return ""
	}

	if params.MDRenderer != nil {
		return renderMarkdownContent(params.MDRenderer, params.Content)
	}

	return params.Content
}

// renderMarkdownContent renders content through the markdown renderer.
func renderMarkdownContent(mdRenderer *MDRenderer, content string) string {
	rendered, err := mdRenderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}

// getToolExecutionDesc returns a human-readable description for a tool being executed.
func getToolExecutionDesc(toolName string) string {
	switch toolName {
	case tool.ToolExitPlanMode:
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
	case tool.ToolSkill:
		return "Loading skill..."
	default:
		return "Executing..."
	}
}

// RenderSystemMessage renders a system/notice core.
func RenderSystemMessage(content string) string {
	return systemMsgStyle.Render(content) + "\n"
}

// ToolCallsParams holds the parameters for rendering tool calls.
type ToolCallsParams struct {
	ToolCalls         []core.ToolCall
	ToolCallsExpanded bool
	ResultMap         map[string]ToolResultData
	ParallelMode      bool
	ParallelResults   map[int]bool
	TaskProgress      map[int][]string
	PendingCalls      []core.ToolCall
	CurrentIdx        int
	SpinnerView       string
	TaskOwnerMap      map[string]string
	MDRenderer        *MDRenderer
	Width             int
}

// ToolResultData holds the data needed to render a tool result inline.
type ToolResultData struct {
	ToolName  string
	Content   string
	Error     string
	IsError   bool
	Expanded  bool
	ToolInput string
}

// RenderToolCalls renders the tool calls section of an assistant core.
func RenderToolCalls(params ToolCallsParams) string {
	var sb strings.Builder

	for _, tc := range params.ToolCalls {
		switch tc.Name {
		case tool.ToolTaskList, tool.ToolTaskCreate, tool.ToolTaskUpdate:
			continue
		}
		if tool.IsAgentToolName(tc.Name) {
			label := formatAgentLabel(tc.Input)
			_, hasResult := params.ResultMap[tc.ID]
			if hasResult {
				sb.WriteString(renderToolLine(label, params.Width) + "\n")
			} else {
				sb.WriteString(renderToolLineWithIcon(label, params.Width, params.SpinnerView))
				if !params.ToolCallsExpanded {
					sb.WriteString(ThinkingStyle.Render("  (ctrl+o to expand)"))
				}
				sb.WriteString("\n")
			}
			if params.ToolCallsExpanded && !hasResult {
				sb.WriteString(formatAgentDefinition(tc.Input))
			}
		} else if params.ToolCallsExpanded {
			toolLine := renderToolLine(tc.Name, params.Width)
			sb.WriteString(toolLine + "\n")
			var p map[string]any
			if err := json.Unmarshal([]byte(tc.Input), &p); err == nil {
				for k, v := range p {
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
			icon := toolCallIcon(tc, params.PendingCalls, params.CurrentIdx, params.ParallelMode, params.ParallelResults, params.SpinnerView)
			if tc.Name == tool.ToolTaskGet && params.TaskOwnerMap != nil {
				args := extractTaskGetDisplay(tc.Input, params.TaskOwnerMap)
				sb.WriteString(renderToolLineWithIcon(fmt.Sprintf("%s(%s)", tc.Name, args), params.Width, icon) + "\n")
			} else {
				args := extractToolArgs(tc.Input)
				sb.WriteString(renderToolLineWithIcon(fmt.Sprintf("%s(%s)", tc.Name, args), params.Width, icon) + "\n")
			}
		}

		if resultData, ok := params.ResultMap[tc.ID]; ok {
			resultData.ToolInput = tc.Input
			sb.WriteString(RenderToolResultInline(resultData, params.MDRenderer))
		} else if params.ParallelMode && tool.IsAgentToolName(tc.Name) {
			sb.WriteString(renderTaskProgressInline(tc, params.PendingCalls, params.ParallelResults, params.TaskProgress))
		}
	}

	return sb.String()
}

func toolCallIcon(tc core.ToolCall, pendingCalls []core.ToolCall, currentIdx int, parallelMode bool, parallelResults map[int]bool, spinnerView string) string {
	idx := -1
	for i, pending := range pendingCalls {
		if pending.ID == tc.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "●"
	}

	if parallelMode {
		if _, done := parallelResults[idx]; !done {
			return spinnerView
		}
		return "●"
	}

	if idx == currentIdx {
		return spinnerView
	}

	return "●"
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

// RenderToolResultInline renders a tool result inline (without leading newline).
func RenderToolResultInline(data ToolResultData, mdRenderer *MDRenderer) string {
	toolName := data.ToolName
	if toolName == "" {
		toolName = "Tool"
	}

	switch toolName {
	case tool.ToolSkill:
		return renderSkillResultInline(data)
	case tool.ToolAgent, tool.ToolContinueAgent, tool.ToolSendMessage:
		return renderTaskResultInline(data, mdRenderer)
	case tool.ToolTaskOutput:
		return renderTaskOutputResultInline(data)
	case tool.ToolAskUserQuestion:
		return renderAskUserResultInline(data)
	}

	sizeInfo := formatToolResultSize(toolName, data.Content)
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s → %s", icon, toolName, sizeInfo))
	sb.WriteString(summary + "\n")

	if data.Expanded || data.IsError {
		for line := range strings.SplitSeq(data.Content, "\n") {
			sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// renderAskUserResultInline renders AskUserQuestion result with answer summary.
func renderAskUserResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	if data.IsError {
		return toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, data.Content)) + "\n"
	}

	if strings.Contains(data.Content, "User cancelled") {
		return toolResultStyle.Render(fmt.Sprintf("  %s  Cancelled", icon)) + "\n"
	}

	var answers []string
	for line := range strings.SplitSeq(data.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "User responses:" {
			continue
		}
		if answers == nil {
			answers = make([]string, 0, 4)
		}
		answers = append(answers, line)
	}

	if len(answers) == 0 {
		return toolResultStyle.Render(fmt.Sprintf("  %s  Answered", icon)) + "\n"
	}

	var sb strings.Builder
	for _, a := range answers {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, a)) + "\n")
	}
	return sb.String()
}

// renderSkillResultInline renders a skill result with clean formatting.
func renderSkillResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	if data.IsError {
		summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, data.Content))
		sb.WriteString(summary + "\n")
		return sb.String()
	}

	skillName, scriptCount, refCount := parseSkillResultContent(data.Content)
	resources := make([]string, 0, 2)
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

	summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s", icon, result))
	sb.WriteString(summary + "\n")

	if data.Expanded {
		for line := range strings.SplitSeq(data.Content, "\n") {
			sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

// renderTaskResultInline renders a Task tool result with agent-specific formatting.
func renderTaskResultInline(data ToolResultData, mdRenderer *MDRenderer) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content

	if data.IsError {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  Agent → Error", icon)) + "\n")
		sb.WriteString(toolResultExpandedStyle.Render("    "+content) + "\n")
		return sb.String()
	}

	taskID := extractField(content, "Task ID: ", "")
	isBackground := strings.Contains(content, "started in background")
	if isBackground && taskID != "" {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  → background (Task ID: %s)", icon, taskID)) + "\n")
		return sb.String()
	}

	toolUses := extractIntField(content, "ToolUses: ")
	tokens := extractIntField(content, "Tokens: ")
	duration := extractField(content, "Duration: ", "")
	resultModel := extractField(content, "Model: ", "\n")
	doneStats := buildDoneStats(toolUses, tokens, duration, resultModel)

	if !data.Expanded {
		resultLine := fmt.Sprintf("  %s  Done", icon)
		if doneStats != "" {
			resultLine += " (" + doneStats + ")"
		}
		sb.WriteString(toolResultStyle.Render(resultLine))
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
	processCount := extractIntField(content, "Process: ")
	process, response := splitByProcessCount(body, processCount)

	if process != "" {
		for line := range strings.SplitSeq(process, "\n") {
			sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  ⎿  %s", line)) + "\n")
		}
	}

	if response != "" {
		sb.WriteString(agentLabelStyle.Render("  ⎿  Response:") + "\n")
		rendered := response
		if mdRenderer != nil {
			narrowRenderer := NewMDRenderer(mdRenderer.width - len(agentContentIndent))
			if md, err := narrowRenderer.Render(response); err == nil {
				rendered = strings.TrimSpace(md)
			}
		}
		for line := range strings.SplitSeq(rendered, "\n") {
			sb.WriteString(toolResultExpandedStyle.Render(agentContentIndent+line) + "\n")
		}
	}

	resultLine := "  ⎿  Done"
	if doneStats != "" {
		resultLine += " (" + doneStats + ")"
	}
	sb.WriteString(toolResultStyle.Render(resultLine) + "\n")
	return sb.String()
}

// renderTaskOutputResultInline renders a TaskOutput result with task-specific formatting.
func renderTaskOutputResultInline(data ToolResultData) string {
	icon := toolResultIcon(data.IsError)

	var sb strings.Builder
	content := data.Content
	errorText := data.Error
	if errorText == "" {
		errorText = content
	}

	if data.IsError {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  %s  TaskOutput → Error", icon)) + "\n")
		if errorText != "" {
			sb.WriteString(toolResultExpandedStyle.Render("    "+errorText) + "\n")
		}
		return sb.String()
	}

	agentName := extractField(content, "Agent: ", "")
	status := extractField(content, "Status: ", "")
	turns := extractIntField(content, "Turns: ")

	var info []string
	if agentName != "" {
		info = make([]string, 0, 3)
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

	if _, outputContent, found := strings.Cut(content, "Output:\n"); found {
		outputLines := strings.Split(outputContent, "\n")
		maxLines := 10
		if data.Expanded {
			maxLines = len(outputLines)
		}

		for i, line := range outputLines {
			if i >= maxLines {
				sb.WriteString(toolResultExpandedStyle.Render("    ...") + "\n")
				break
			}
			sb.WriteString(toolResultExpandedStyle.Render("    "+stripMarkdownHeading(line)) + "\n")
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
	meta := make([]string, 0, 2)
	if mode, ok := params["mode"].(string); ok && mode != "" {
		meta = append(meta, fmt.Sprintf("mode=%s", mode))
	}
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		meta = append(meta, "background")
	}
	if len(meta) > 0 {
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  ⎿  [%s]", strings.Join(meta, ", "))) + "\n")
	}

	if prompt, ok := params["prompt"].(string); ok && prompt != "" {
		sb.WriteString(agentLabelStyle.Render("  ⎿  Prompt:") + "\n")
		for line := range strings.SplitSeq(prompt, "\n") {
			sb.WriteString(toolResultExpandedStyle.Render(agentContentIndent+line) + "\n")
		}
	}

	return sb.String()
}

// buildDoneStats builds the stats string for the Done line.
func buildDoneStats(toolUses, tokens int, duration, model string) string {
	stats := make([]string, 0, 4)
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

// parseSkillResultContent extracts skill info from skill-invocation content.
func parseSkillResultContent(content string) (skillName string, scriptCount, refCount int) {
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

// extractField extracts a field value from content by prefix, returning defaultVal if not found.
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

// extractIntField extracts an integer field value from content by prefix.
func extractIntField(content, prefix string) int {
	val := extractField(content, prefix, "")
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

// formatAgentLabel formats an Agent tool call as "Agent: AgentType description".
func formatAgentLabel(input string) string {
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

// extractToolArgs extracts the most relevant argument from a tool call input JSON.
func extractToolArgs(input string) string {
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

// formatToolResultSize returns a human-readable size description for a tool result.
func formatToolResultSize(toolName, content string) string {
	switch toolName {
	case "WebFetch":
		return toolresult.FormatSize(int64(len(content)))
	case "Write", "Edit":
		return extractParenContent(content, "completed")
	default:
		return formatLineCount(content)
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
	icon := toolCallStyle.Width(2).Render(iconText)
	return lipgloss.JoinHorizontal(lipgloss.Top, icon, toolCallStyle.Render(truncateToolLabel(label, width)))
}

func truncateToolLabel(label string, width int) string {
	maxWidth := maxToolLabelWidth(width)
	if lipgloss.Width(label) <= maxWidth {
		return label
	}
	return truncateText(label, maxWidth)
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

// QueuePreviewItem is the minimal data needed to render a queue item preview.
type QueuePreviewItem struct {
	Content   string
	HasImages bool
}

var (
	queueBadgeStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Accent).
			Bold(true)

	queueContentStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.TextDim)

	queueSelectedBadgeStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.TextBright).
				Bold(true)

	queueSelectedContentStyle = lipgloss.NewStyle().
					Foreground(kit.CurrentTheme.Text)

	queueOverflowStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Muted).
				Italic(true)
)

// RenderQueuePreview renders queued input items above the input area.
// selectedIdx is the currently selected item index (-1 = none).
func RenderQueuePreview(items []QueuePreviewItem, selectedIdx, width int) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder

	maxVisible := 5
	startIdx := 0
	if len(items) > maxVisible && selectedIdx >= maxVisible {
		startIdx = selectedIdx - maxVisible + 1
	}
	endIdx := min(startIdx+maxVisible, len(items))

	for i := startIdx; i < endIdx; i++ {
		item := items[i]
		isSelected := i == selectedIdx

		content := truncateQueueContent(item.Content, width-8)
		if item.HasImages {
			content = PendingImageStyle.Render("[Image] ") + content
		}

		if isSelected {
			badge := queueSelectedBadgeStyle.Render(fmt.Sprintf("▸ %d.", i+1))
			preview := queueSelectedContentStyle.Render(content)
			fmt.Fprintf(&sb, " %s %s\n", badge, preview)
		} else {
			badge := queueBadgeStyle.Render(fmt.Sprintf("  %d.", i+1))
			preview := queueContentStyle.Render(content)
			fmt.Fprintf(&sb, " %s %s\n", badge, preview)
		}
	}

	if len(items) > maxVisible {
		if endIdx < len(items) {
			sb.WriteString(queueOverflowStyle.Render(fmt.Sprintf("     +%d more below", len(items)-endIdx)) + "\n")
		}
		if startIdx > 0 {
			above := queueOverflowStyle.Render(fmt.Sprintf("     +%d more above", startIdx)) + "\n"
			return above + sb.String()
		}
	}

	return sb.String()
}

// renderQueueBadge renders a compact badge for the status bar.
func renderQueueBadge(count int) string {
	if count == 0 {
		return ""
	}
	return queueBadgeStyle.Render(fmt.Sprintf(" [%d queued]", count))
}

func truncateQueueContent(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")

	if maxLen <= 0 {
		maxLen = 40
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

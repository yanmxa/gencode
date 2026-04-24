// Pure message rendering functions that take explicit parameters instead of model state.
package conv

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
)

// OperationMode mirrors OperationMode to avoid importing setting in the render layer.
type OperationMode int

const (
	ModeNormal OperationMode = iota
	ModeAutoAccept
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
	Mode             OperationMode
	InputTokens      int
	OutputTokens     int
	InputLimit       int
	ModelName        string
	ConversationCost llm.Money
	Width            int
	ThinkingLevel    llm.ThinkingLevel
	QueueCount       int
}

// RenderModeStatus renders the combined mode status line.
func RenderModeStatus(params OperationModeParams) string {
	var leftParts []string

	if modeStatus := RenderOperationModeIndicator(params.Mode); modeStatus != "" {
		leftParts = append(leftParts, modeStatus)
	}

	if thinkingStatus := RenderThinkingIndicator(params.ThinkingLevel); thinkingStatus != "" {
		leftParts = append(leftParts, thinkingStatus)
	}

	if tokenWarning := renderTokenWarningInline(params.InputTokens, params.InputLimit); tokenWarning != "" {
		leftParts = append(leftParts, tokenWarning)
	}

	if queueBadge := renderQueueBadge(params.QueueCount); queueBadge != "" {
		leftParts = append(leftParts, queueBadge)
	}

	left := strings.Join(leftParts, "  ")

	right := renderModelWithTokens(params.ModelName, params.InputTokens, params.InputLimit, params.ConversationCost)
	if right == "" || params.Width <= 0 {
		return left
	}

	gap := max(2, params.Width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	return left + strings.Repeat(" ", gap) + right
}

// renderModelWithTokens renders the model name with token usage on the right side.
func renderModelWithTokens(modelName string, inputTokens, inputLimit int, conversationCost llm.Money) string {
	if modelName == "" {
		return ""
	}
	muted := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	parts := []string{modelName}

	if inputTokens == 0 {
		if !conversationCost.IsZero() {
			parts = append(parts, kit.FormatMoney(conversationCost))
		}
		return muted.Render(strings.Join(parts, " · "))
	}

	if inputLimit > 0 {
		pct := float64(inputTokens) / float64(inputLimit) * 100
		parts = append(parts, fmt.Sprintf("%.0f%% ctx", pct))
	}
	if !conversationCost.IsZero() {
		parts = append(parts, kit.FormatMoney(conversationCost))
	}
	return muted.Render(strings.Join(parts, " · "))
}

func RenderTurnUsageSummary(inputTokens, outputTokens, width int) string {
	if inputTokens == 0 && outputTokens == 0 {
		return ""
	}

	summary := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Render(
		fmt.Sprintf("↑%s ↓%s", kit.FormatTokenCount(inputTokens), kit.FormatTokenCount(outputTokens)),
	)
	if width <= 0 {
		return summary
	}

	gap := max(0, width-lipgloss.Width(summary))
	return strings.Repeat(" ", gap) + summary
}

// RenderOperationModeIndicator returns the mode status indicator for auto-accept or bypass mode.
func RenderOperationModeIndicator(mode OperationMode) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch mode {
	case ModeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = kit.CurrentTheme.Success
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

// renderTokenWarningInline returns a warning when context usage exceeds 50%.
func renderTokenWarningInline(inputTokens, inputLimit int) string {
	if inputLimit == 0 || inputTokens == 0 {
		return ""
	}

	percent := float64(inputTokens) / float64(inputLimit) * 100
	if percent < 50 {
		return ""
	}

	color, hint := tokenUsageColorAndHint(percent)
	style := lipgloss.NewStyle().Foreground(color)

	used := kit.FormatTokenCount(inputTokens)
	limit := kit.FormatTokenCount(inputLimit)
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
		if displayContent != "" {
			sb.WriteString(prompt + imageLabel + " " + userMsgStyle.Render(displayContent) + "\n")
		} else {
			sb.WriteString(prompt + imageLabel + "\n")
		}
	} else if displayContent != "" {
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, prompt, userMsgStyle.Render(displayContent)) + "\n")
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
				sb.WriteString(formatAgentDefinition(tc.Input, params.Width))
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
			if _, hasResult := params.ResultMap[tc.ID]; hasResult {
				icon = "●"
			}
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

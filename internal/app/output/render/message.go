// Pure message rendering functions that take explicit parameters instead of model state.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/app/kit"
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
	Mode          config.OperationMode
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
func RenderOperationModeIndicator(mode config.OperationMode) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch mode {
	case config.ModeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = kit.CurrentTheme.Success
	case config.ModePlan:
		icon = "⏸"
		label = " plan mode on"
		color = kit.CurrentTheme.Warning
	case config.ModeBypassPermissions:
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

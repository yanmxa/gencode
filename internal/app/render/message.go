// Pure message rendering functions that take explicit parameters instead of model state.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

const (
	// MinWrapWidth is the minimum markdown wrap width.
	MinWrapWidth = 40

	// AutoCompactThreshold is the percentage of context usage that triggers auto-compact.
	AutoCompactThreshold = 95

	// agentContentIndent is the extra indent for agent prompt/response content
	// beyond ToolResultExpandedStyle's PaddingLeft(4). Total indent = 4 + 4 = 8 chars.
	agentContentIndent = "    "
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
	Mode          int
	InputTokens   int
	InputLimit    int
	ModelName     string
	Width         int
	ThinkingLevel provider.ThinkingLevel
}

// RenderModeStatus renders the combined mode status line.
func RenderModeStatus(params OperationModeParams) string {
	var parts []string

	if modeStatus := RenderOperationModeIndicator(params.Mode); modeStatus != "" {
		parts = append(parts, modeStatus)
	}

	if thinkingStatus := RenderThinkingIndicator(params.ThinkingLevel); thinkingStatus != "" {
		parts = append(parts, thinkingStatus)
	}

	if tokenUsage := RenderTokenUsage(params.InputTokens, params.InputLimit); tokenUsage != "" {
		parts = append(parts, tokenUsage)
	}

	left := strings.Join(parts, "  ")

	if params.ModelName == "" || params.Width <= 0 {
		return left
	}

	modelStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	right := modelStyle.Render(params.ModelName)
	gap := max(2, params.Width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	return left + strings.Repeat(" ", gap) + right
}

// RenderOperationModeIndicator returns the mode status indicator for auto-accept or plan mode.
func RenderOperationModeIndicator(mode int) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch appmode.OperationMode(mode) {
	case appmode.AutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = theme.CurrentTheme.Success
	case appmode.Plan:
		icon = "⏸"
		label = " plan mode on"
		color = theme.CurrentTheme.Warning
	case appmode.BypassPermissions:
		icon = "⏩"
		label = " bypass permissions on"
		color = theme.CurrentTheme.Error
	default:
		return ""
	}

	style := lipgloss.NewStyle().Foreground(color)
	hint := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Render("  shift+tab to toggle")
	return "  " + style.Render(icon+label) + hint
}

// RenderThinkingIndicator returns a styled indicator for the current thinking level.
func RenderThinkingIndicator(level provider.ThinkingLevel) string {
	var icon, label string
	var color lipgloss.TerminalColor

	switch level {
	case provider.ThinkingNormal:
		icon = "✦"
		label = " think"
		color = theme.CurrentTheme.Accent
	case provider.ThinkingHigh:
		icon = "✦✦"
		label = " think+"
		color = theme.CurrentTheme.Primary
	case provider.ThinkingUltra:
		icon = "✦✦✦"
		label = " ultrathink"
		color = theme.CurrentTheme.AI
	default:
		return ""
	}

	style := lipgloss.NewStyle().Foreground(color)
	return "  " + style.Render(icon+label)
}

// RenderTokenUsage returns token usage indicator.
func RenderTokenUsage(inputTokens, inputLimit int) string {
	if inputLimit == 0 || inputTokens == 0 {
		return ""
	}

	percent := float64(inputTokens) / float64(inputLimit) * 100
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
func TokenUsageColorAndHint(percent float64) (lipgloss.TerminalColor, string) {
	if percent >= AutoCompactThreshold {
		return theme.CurrentTheme.Error, " ⚠ auto-compact"
	}
	if percent >= 85 {
		return theme.CurrentTheme.Warning, fmt.Sprintf(" (compact at %d%%)", AutoCompactThreshold)
	}
	if percent >= 70 {
		return theme.CurrentTheme.Accent, ""
	}
	return theme.CurrentTheme.Muted, ""
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

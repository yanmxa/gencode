package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
)

func RenderCompactStatus(width int, spinnerView string, active bool, focus, phase, result string, isError bool) string {
	if !active && result == "" {
		return ""
	}

	label := "SESSION SUMMARY"
	title := "Conversation compacted"
	subtitle := "Older context was folded into a shorter summary. You can continue normally."
	detail := result
	accent := kit.CurrentTheme.Success
	icon := "✓"

	if active {
		if phase != "" {
			title = spinnerView + " " + phase
		} else {
			title = spinnerView + " Compacting conversation"
		}
		subtitle = "Summarizing recent history into a shorter reusable summary."
		if strings.TrimSpace(focus) != "" {
			detail = "Focus: " + focus
		} else {
			detail = "Preparing a smaller conversation state for the next turns."
		}
		accent = kit.CurrentTheme.Primary
		icon = ""
	} else if isError {
		label = "COMPACT ERROR"
		title = "Compact failed"
		subtitle = "Conversation history was not replaced. You can retry once the issue is resolved."
		accent = kit.CurrentTheme.Error
		icon = "✗"
	}

	if icon != "" {
		title = icon + " " + title
	}

	boxWidth := kit.CalculateBoxWidth(width)

	labelStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim).
		Bold(true)
	headerStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	subtitleStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.Text)
	bodyStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(kit.CurrentTheme.Background).
		Padding(0, 1).
		Width(boxWidth).
		MarginLeft(1)

	var lines []string
	lines = append(lines, labelStyle.Render(label))
	lines = append(lines, headerStyle.Render(title))
	if strings.TrimSpace(subtitle) != "" {
		lines = append(lines, subtitleStyle.Render(subtitle))
	}
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, bodyStyle.Render(detail))
	}

	return boxStyle.Render(strings.Join(lines, "\n"))
}

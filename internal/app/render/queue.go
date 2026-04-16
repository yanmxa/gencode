package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	appqueue "github.com/yanmxa/gencode/internal/app/queue"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	queueBadgeStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Accent).
			Bold(true)

	queueContentStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDim)

	queueSelectedBadgeStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextBright).
				Bold(true)

	queueSelectedContentStyle = lipgloss.NewStyle().
					Foreground(theme.CurrentTheme.Text)

	queueOverflowStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Muted).
				Italic(true)
)

// RenderQueuePreview renders queued input items above the input area.
// selectedIdx is the currently selected item index (-1 = none).
func RenderQueuePreview(items []appqueue.Item, selectedIdx, width int) string {
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
		if len(item.Images) > 0 {
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

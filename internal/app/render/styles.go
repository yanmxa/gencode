package render

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	userMsgStyle      = lipgloss.NewStyle()
	assistantMsgStyle = lipgloss.NewStyle()

	InputPromptStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary).
				Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.AI).
			Bold(true)

	SeparatorStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Separator)

	ThinkingStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Muted)

	systemMsgStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.TextDim).
			PaddingLeft(2)

	toolCallStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Text)

	toolResultStyle = toolCallStyle

	toolResultExpandedStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDim).
				PaddingLeft(4)

	agentLabelStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Success)

	trackerPendingStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Muted)

	trackerInProgressStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary).
				Bold(true)

	trackerCompletedStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDisabled).
				Strikethrough(true)

	PendingImageStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary)

	pendingImageHintStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Muted)

	SelectedImageStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextBright).
				Background(theme.CurrentTheme.Primary).
				Bold(true)
)

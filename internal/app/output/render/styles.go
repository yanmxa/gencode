package render

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
)

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

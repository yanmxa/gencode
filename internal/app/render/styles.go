package render

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	UserMsgStyle      = lipgloss.NewStyle()
	AssistantMsgStyle = lipgloss.NewStyle()

	InputPromptStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary).
				Bold(true)

	AIPromptStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.AI).
			Bold(true)

	SeparatorStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Separator)

	ThinkingStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Muted)

	SystemMsgStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.TextDim).
			PaddingLeft(2)

	ToolCallStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Text)

	ToolResultStyle = ToolCallStyle

	ToolResultExpandedStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDim).
				PaddingLeft(4)

	AgentLabelStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Success)

	TrackerPendingStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Muted)

	TrackerInProgressStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary).
				Bold(true)

	TrackerCompletedStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDisabled).
				Strikethrough(true)

	PendingImageStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary)

	PendingImageHintStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Muted)

	SelectedImageStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextBright).
				Background(theme.CurrentTheme.Primary).
				Bold(true)
)

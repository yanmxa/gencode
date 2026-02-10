package tui

import "github.com/charmbracelet/lipgloss"

// Message styles
var (
	userMsgStyle         lipgloss.Style
	assistantMsgStyle    lipgloss.Style
	inputPromptStyle     lipgloss.Style
	aiPromptStyle        lipgloss.Style
	separatorStyle       lipgloss.Style
	thinkingStyle        lipgloss.Style
	thinkingContentStyle lipgloss.Style // for reasoning content display
	systemMsgStyle       lipgloss.Style
)

// Tool display styles
var (
	toolCallStyle           lipgloss.Style
	toolResultStyle         lipgloss.Style
	toolResultExpandedStyle lipgloss.Style
)

// Todo styles
var (
	todoPendingStyle    lipgloss.Style
	todoInProgressStyle lipgloss.Style
	todoCompletedStyle  lipgloss.Style
)

// Image styles
var (
	pendingImageStyle     lipgloss.Style
	pendingImageHintStyle lipgloss.Style
	selectedImageStyle    lipgloss.Style
)

func init() {
	// Message styles
	userMsgStyle = lipgloss.NewStyle()
	assistantMsgStyle = lipgloss.NewStyle()

	inputPromptStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary).
		Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.AI).
		Bold(true)

	separatorStyle = lipgloss.NewStyle().
		Faint(true).
		Foreground(CurrentTheme.Separator)

	thinkingStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Accent)

	// Thinking content style - slightly dimmed for reasoning content
	thinkingContentStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	systemMsgStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim).
		PaddingLeft(2)

	// Tool display styles
	toolCallStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Accent)

	toolResultStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	toolResultExpandedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim).
		PaddingLeft(4)

	// Todo styles
	todoPendingStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	todoInProgressStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Warning).
		Bold(true)

	todoCompletedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDisabled).
		Strikethrough(true)

	// Image styles
	pendingImageStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary)

	pendingImageHintStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	selectedImageStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextBright).
		Background(CurrentTheme.Primary).
		Bold(true)
}

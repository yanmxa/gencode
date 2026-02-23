// Lipgloss style definitions for messages, tools, todos, and images.
package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tui/theme"
)

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

var (
	SelectorBorderStyle     lipgloss.Style
	SelectorTitleStyle      lipgloss.Style
	SelectorItemStyle       lipgloss.Style
	SelectorSelectedStyle   lipgloss.Style
	SelectorStatusConnected lipgloss.Style
	SelectorStatusReady     lipgloss.Style
	SelectorStatusNone      lipgloss.Style
	SelectorStatusError     lipgloss.Style
	SelectorHintStyle       lipgloss.Style
	SelectorBreadcrumbStyle lipgloss.Style
)

func init() {
	// Message styles
	userMsgStyle = lipgloss.NewStyle()
	assistantMsgStyle = lipgloss.NewStyle()

	inputPromptStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.AI).
		Bold(true)

	separatorStyle = lipgloss.NewStyle().
		Faint(true).
		Foreground(theme.CurrentTheme.Separator)

	thinkingStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Accent)

	// Thinking content style - slightly dimmed for reasoning content
	thinkingContentStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	systemMsgStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim).
		PaddingLeft(2)

	// Tool display styles
	toolCallStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Accent)

	toolResultStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	toolResultExpandedStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim).
		PaddingLeft(4)

	// Todo styles
	todoPendingStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	todoInProgressStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Warning).
		Bold(true)

	todoCompletedStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDisabled).
		Strikethrough(true)

	// Image styles
	pendingImageStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary)

	pendingImageHintStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	selectedImageStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Background(theme.CurrentTheme.Primary).
		Bold(true)

	SelectorBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.CurrentTheme.Primary).
		Padding(1, 2)

	SelectorTitleStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)

	SelectorItemStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted).
		PaddingLeft(2)

	SelectorSelectedStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Bold(true).
		PaddingLeft(2)

	SelectorStatusConnected = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Success)

	SelectorStatusReady = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Warning)

	SelectorStatusNone = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	SelectorStatusError = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9"))

	SelectorHintStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted).
		MarginTop(1)

	SelectorBreadcrumbStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim).
		MarginBottom(1)
}

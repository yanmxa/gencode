// Lipgloss style definitions for messages, tools, todos, and images.
package render

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Message styles
var (
	UserMsgStyle         lipgloss.Style
	AssistantMsgStyle    lipgloss.Style
	InputPromptStyle     lipgloss.Style
	AIPromptStyle        lipgloss.Style
	SeparatorStyle       lipgloss.Style
	ThinkingStyle        lipgloss.Style
	ThinkingContentStyle lipgloss.Style // for reasoning content display
	SystemMsgStyle       lipgloss.Style
)

// Tool display styles
var (
	ToolCallStyle           lipgloss.Style
	ToolResultStyle         lipgloss.Style
	ToolResultExpandedStyle lipgloss.Style
	AgentLabelStyle         lipgloss.Style // Labels like "Prompt:" and "Response:" in agent results
)

// Todo styles
var (
	TodoPendingStyle    lipgloss.Style
	TodoInProgressStyle lipgloss.Style
	TodoCompletedStyle  lipgloss.Style
)

// Image styles
var (
	PendingImageStyle     lipgloss.Style
	PendingImageHintStyle lipgloss.Style
	SelectedImageStyle    lipgloss.Style
)

func init() {
	// Message styles
	UserMsgStyle = lipgloss.NewStyle()
	AssistantMsgStyle = lipgloss.NewStyle()

	InputPromptStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)

	AIPromptStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.AI).
		Bold(true)

	SeparatorStyle = lipgloss.NewStyle().
		Faint(true).
		Foreground(theme.CurrentTheme.Separator)

	ThinkingStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	ThinkingContentStyle = lipgloss.NewStyle().
		Faint(true)

	SystemMsgStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim).
		PaddingLeft(2)

	// Tool display styles — unified muted color family
	ToolCallStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim)

	ToolResultStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	ToolResultExpandedStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted).
		PaddingLeft(4)

	AgentLabelStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Success)

	// Todo styles
	TodoPendingStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	TodoInProgressStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)

	TodoCompletedStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDisabled).
		Strikethrough(true)

	// Image styles
	PendingImageStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary)

	PendingImageHintStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	SelectedImageStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Background(theme.CurrentTheme.Primary).
		Bold(true)
}

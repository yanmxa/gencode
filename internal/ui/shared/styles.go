package shared

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Selector styles shared across feature packages.
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

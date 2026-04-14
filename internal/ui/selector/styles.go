package selector

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	SelectorBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.CurrentTheme.Primary).
				Padding(1, 2)

	SelectorTitleStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Primary).
				Bold(true)

	SelectorItemStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Text).
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
				Foreground(theme.CurrentTheme.TextDim)

	SelectorStatusError = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Error)

	SelectorHintStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDim).
				MarginTop(1)

	SelectorBreadcrumbStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Text).
				MarginBottom(1)

	// SelectorDimStyle is a plain dim-text style (no margins/padding).
	SelectorDimStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.TextDim)
)

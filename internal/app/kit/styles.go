package kit

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	SelectorBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(CurrentTheme.Primary).
				Padding(1, 2)

	SelectorTitleStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.Primary).
				Bold(true)

	SelectorItemStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.Text).
				PaddingLeft(2)

	SelectorSelectedStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.TextBright).
				Bold(true).
				PaddingLeft(2)

	SelectorStatusConnected = lipgloss.NewStyle().
				Foreground(CurrentTheme.Success)

	SelectorStatusReady = lipgloss.NewStyle().
				Foreground(CurrentTheme.Warning)

	SelectorStatusNone = lipgloss.NewStyle().
				Foreground(CurrentTheme.TextDim)

	SelectorStatusError = lipgloss.NewStyle().
				Foreground(CurrentTheme.Error)

	SelectorHintStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.TextDim).
				MarginTop(1)

	SelectorBreadcrumbStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.Text).
				MarginBottom(1)

	// SelectorDimStyle is a plain dim-text style (no margins/padding).
	SelectorDimStyle = lipgloss.NewStyle().
				Foreground(CurrentTheme.TextDim)

	// TabActiveBg is the background color for active tabs in tabbed panels.
	TabActiveBg = lipgloss.AdaptiveColor{Dark: "#4F6D9B", Light: "#3B6FC0"}
	// TabActiveFg is the foreground color for active tabs in tabbed panels.
	TabActiveFg = lipgloss.AdaptiveColor{Dark: "#FFFFFF", Light: "#FFFFFF"}
	// SearchBg is the background color for search/filter input boxes.
	SearchBg = lipgloss.AdaptiveColor{Dark: "#27272A", Light: "#E4E4E7"}
)

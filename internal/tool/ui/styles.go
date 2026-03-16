package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	ColorSuccess = theme.CurrentTheme.Success
	ColorError   = theme.CurrentTheme.Error
	ColorMuted   = theme.CurrentTheme.Muted
	ColorAccent  = theme.CurrentTheme.Primary
	ColorWarn    = theme.CurrentTheme.Warning
	ColorMatch   = theme.CurrentTheme.Warning
	ColorBorder  = theme.CurrentTheme.Border
)

const (
	IconRead     = "\U0001F4C4" // 📄
	IconGlob     = "\U0001F50D" // 🔍
	IconGrep     = "\U0001F50E" // 🔎
	IconWeb      = "\U0001F310" // 🌐
	IconError    = "\u274C"     // ❌
	IconSuccess  = "\u2713"     // ✓
	IconFile     = "\U0001F4C1" // 📁
	IconDuration = "\u23F1"     // ⏱
)

var (
	HeaderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	HeaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent)

	HeaderSubtitleStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Text)

	HeaderMetaStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	LineNumberStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(5).
			Align(lipgloss.Right)

	MatchStyle = lipgloss.NewStyle().
			Foreground(ColorMatch).
			Bold(true)

	FilePathStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	TruncatedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError)

	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorWarn)

	ProgressMsgStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)
)

func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func FormatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
}

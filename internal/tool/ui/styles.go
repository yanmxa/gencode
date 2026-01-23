package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	ColorSuccess = lipgloss.Color("#10B981") // green
	ColorError   = lipgloss.Color("#EF4444") // red
	ColorMuted   = lipgloss.Color("#6B7280") // gray
	ColorAccent  = lipgloss.Color("#60A5FA") // blue
	ColorWarn    = lipgloss.Color("#F59E0B") // yellow
	ColorMatch   = lipgloss.Color("#FBBF24") // yellow (highlight match)
	ColorBorder  = lipgloss.Color("#374151") // dark gray for borders
)

// Icons
const (
	IconRead     = "\U0001F4C4" // 📄
	IconGlob     = "\U0001F50D" // 🔍
	IconGrep     = "\U0001F50E" // 🔎
	IconWeb      = "\U0001F310" // 🌐
	IconSearch   = "\U0001F50D" // 🔍 (web search)
	IconError    = "\u274C"     // ❌
	IconSuccess  = "\u2713"     // ✓
	IconFile     = "\U0001F4C1" // 📁
	IconDuration = "\u23F1"     // ⏱
)

// Styles
var (
	// Header box styles
	HeaderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	HeaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent)

	HeaderSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5E7EB"))

	HeaderMetaStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Content styles
	LineNumberStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(5).
			Align(lipgloss.Right)

	LineContentStyle = lipgloss.NewStyle()

	MatchStyle = lipgloss.NewStyle().
			Foreground(ColorMatch).
			Bold(true)

	FilePathStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	TruncatedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Error styles
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError)

	ErrorMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FCA5A5"))

	// Progress styles
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorWarn)

	ProgressMsgStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)
)

// FormatSize formats bytes to human readable size
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

// FormatDuration formats duration to human readable string
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

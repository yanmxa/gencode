package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme holds all color definitions for the UI
type Theme struct {
	// Base colors
	Muted     lipgloss.Color // muted text, placeholders
	Accent    lipgloss.Color // spinner, highlights
	Primary   lipgloss.Color // user prompt, links
	AI        lipgloss.Color // AI responses
	Separator lipgloss.Color // separator lines

	// Text colors
	Text         lipgloss.Color // normal text
	TextDim      lipgloss.Color // dimmed text
	TextBright   lipgloss.Color // bright/highlighted text
	TextDisabled lipgloss.Color // disabled/strikethrough text

	// Semantic colors
	Success lipgloss.Color // green - added, completed
	Error   lipgloss.Color // red - removed, errors
	Warning lipgloss.Color // amber/orange - in progress

	// UI element colors
	Border     lipgloss.Color // borders
	Background lipgloss.Color // backgrounds for badges/boxes
}

// DarkTheme is the color palette for dark terminals
var DarkTheme = Theme{
	Muted:     lipgloss.Color("#6B7280"),
	Accent:    lipgloss.Color("#F59E0B"),
	Primary:   lipgloss.Color("#60A5FA"),
	AI:        lipgloss.Color("#A78BFA"),
	Separator: lipgloss.Color("#4B5563"),

	Text:         lipgloss.Color("#D1D5DB"),
	TextDim:      lipgloss.Color("#9CA3AF"),
	TextBright:   lipgloss.Color("#FFFFFF"),
	TextDisabled: lipgloss.Color("#4B5563"),

	Success: lipgloss.Color("#10B981"),
	Error:   lipgloss.Color("#EF4444"),
	Warning: lipgloss.Color("#FBBF24"), // Brighter amber to distinguish from Accent

	Border:     lipgloss.Color("#374151"),
	Background: lipgloss.Color("#1F2937"),
}

// LightTheme is the color palette for light terminals
var LightTheme = Theme{
	Muted:     lipgloss.Color("#6B7280"),
	Accent:    lipgloss.Color("#D97706"),
	Primary:   lipgloss.Color("#2563EB"),
	AI:        lipgloss.Color("#7C3AED"),
	Separator: lipgloss.Color("#D1D5DB"),

	Text:         lipgloss.Color("#1F2937"),
	TextDim:      lipgloss.Color("#4B5563"),
	TextBright:   lipgloss.Color("#111827"),
	TextDisabled: lipgloss.Color("#9CA3AF"),

	Success: lipgloss.Color("#059669"),
	Error:   lipgloss.Color("#DC2626"),
	Warning: lipgloss.Color("#B45309"), // Deeper amber to distinguish from Accent

	Border:     lipgloss.Color("#E5E7EB"),
	Background: lipgloss.Color("#F3F4F6"),
}

// CurrentTheme holds the active theme based on terminal background
var CurrentTheme Theme

// isDarkBackground caches the result of background detection
var isDarkBackground bool

func init() {
	// Detect terminal background and set theme
	isDarkBackground = lipgloss.HasDarkBackground()
	if isDarkBackground {
		CurrentTheme = DarkTheme
	} else {
		CurrentTheme = LightTheme
	}
}

// IsDarkBackground returns whether the terminal has a dark background
func IsDarkBackground() bool {
	return isDarkBackground
}

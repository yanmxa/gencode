package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme holds all color definitions for the UI.
// AdaptiveColor selects dark or light values at render time based on terminal background.
type Theme struct {
	Muted     lipgloss.AdaptiveColor
	Accent    lipgloss.AdaptiveColor
	Primary   lipgloss.AdaptiveColor
	AI        lipgloss.AdaptiveColor
	Separator lipgloss.AdaptiveColor

	Text         lipgloss.AdaptiveColor
	TextDim      lipgloss.AdaptiveColor
	TextBright   lipgloss.AdaptiveColor
	TextDisabled lipgloss.AdaptiveColor

	Success   lipgloss.AdaptiveColor
	Error     lipgloss.AdaptiveColor
	Warning   lipgloss.AdaptiveColor
	SuccessBg lipgloss.AdaptiveColor
	ErrorBg   lipgloss.AdaptiveColor

	Border     lipgloss.AdaptiveColor
	Background lipgloss.AdaptiveColor
}

var CurrentTheme = Theme{
	Muted:     lipgloss.AdaptiveColor{Dark: "#6B7280", Light: "#4B5563"},
	Accent:    lipgloss.AdaptiveColor{Dark: "#F59E0B", Light: "#D97706"},
	Primary:   lipgloss.AdaptiveColor{Dark: "#60A5FA", Light: "#2563EB"},
	AI:        lipgloss.AdaptiveColor{Dark: "#A78BFA", Light: "#6D28D9"},
	Separator: lipgloss.AdaptiveColor{Dark: "#4B5563", Light: "#9CA3AF"},

	Text:         lipgloss.AdaptiveColor{Dark: "#D1D5DB", Light: "#111827"},
	TextDim:      lipgloss.AdaptiveColor{Dark: "#9CA3AF", Light: "#4B5563"},
	TextBright:   lipgloss.AdaptiveColor{Dark: "#FFFFFF", Light: "#030712"},
	TextDisabled: lipgloss.AdaptiveColor{Dark: "#4B5563", Light: "#6B7280"},

	Success:   lipgloss.AdaptiveColor{Dark: "#10B981", Light: "#059669"},
	Error:     lipgloss.AdaptiveColor{Dark: "#EF4444", Light: "#DC2626"},
	Warning:   lipgloss.AdaptiveColor{Dark: "#FBBF24", Light: "#B45309"},
	SuccessBg: lipgloss.AdaptiveColor{Dark: "#1a2e1a", Light: "#dafbe1"},
	ErrorBg:   lipgloss.AdaptiveColor{Dark: "#2e1a1a", Light: "#ffebe9"},

	Border:     lipgloss.AdaptiveColor{Dark: "#374151", Light: "#D1D5DB"},
	Background: lipgloss.AdaptiveColor{Dark: "#1F2937", Light: "#F3F4F6"},
}

var (
	darkModeSet bool
	darkModeVal bool
)

// Init configures the active theme. Call once at startup with "light" or "dark".
// An empty string leaves lipgloss auto-detection in place.
func Init(t string) {
	if t != "light" && t != "dark" {
		return
	}
	dark := t == "dark"
	darkModeSet, darkModeVal = true, dark
	lipgloss.SetHasDarkBackground(dark)
}

// IsDarkBackground reports whether the terminal has a dark background.
// Uses the value set by Init if available, otherwise falls back to lipgloss auto-detection.
func IsDarkBackground() bool {
	if darkModeSet {
		return darkModeVal
	}
	return lipgloss.HasDarkBackground()
}

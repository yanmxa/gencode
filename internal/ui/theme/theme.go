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
	Muted:     lipgloss.AdaptiveColor{Dark: "#6B7280", Light: "#6B7280"},
	Accent:    lipgloss.AdaptiveColor{Dark: "#94A3B8", Light: "#64748B"},
	Primary:   lipgloss.AdaptiveColor{Dark: "#CBD5E1", Light: "#475569"},
	AI:        lipgloss.AdaptiveColor{Dark: "#A8B3C7", Light: "#64748B"},
	Separator: lipgloss.AdaptiveColor{Dark: "#475569", Light: "#CBD5E1"},

	Text:         lipgloss.AdaptiveColor{Dark: "#D4D4D8", Light: "#18181B"},
	TextDim:      lipgloss.AdaptiveColor{Dark: "#A1A1AA", Light: "#71717A"},
	TextBright:   lipgloss.AdaptiveColor{Dark: "#FAFAFA", Light: "#09090B"},
	TextDisabled: lipgloss.AdaptiveColor{Dark: "#52525B", Light: "#A1A1AA"},

	Success:   lipgloss.AdaptiveColor{Dark: "#86EFAC", Light: "#15803D"},
	Error:     lipgloss.AdaptiveColor{Dark: "#FCA5A5", Light: "#B91C1C"},
	Warning:   lipgloss.AdaptiveColor{Dark: "#FCD34D", Light: "#B45309"},
	SuccessBg: lipgloss.AdaptiveColor{Dark: "#16281d", Light: "#DCFCE7"},
	ErrorBg:   lipgloss.AdaptiveColor{Dark: "#2b1818", Light: "#FEE2E2"},

	Border:     lipgloss.AdaptiveColor{Dark: "#52525B", Light: "#D4D4D8"},
	Background: lipgloss.AdaptiveColor{Dark: "#18181B", Light: "#FAFAFA"},
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

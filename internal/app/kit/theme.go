package kit

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

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
	Muted:     lipgloss.AdaptiveColor{Dark: "#7B8696", Light: "#6B7280"},
	Accent:    lipgloss.AdaptiveColor{Dark: "#9DB5D4", Light: "#64748B"},
	Primary:   lipgloss.AdaptiveColor{Dark: "#D0DFEF", Light: "#475569"},
	AI:        lipgloss.AdaptiveColor{Dark: "#B0C4E0", Light: "#64748B"},
	Separator: lipgloss.AdaptiveColor{Dark: "#4E6580", Light: "#CBD5E1"},

	Text:         lipgloss.AdaptiveColor{Dark: "#DDDEE2", Light: "#18181B"},
	TextDim:      lipgloss.AdaptiveColor{Dark: "#A8AEBB", Light: "#71717A"},
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

func InitTheme(t string) {
	if t != "light" && t != "dark" {
		return
	}
	dark := t == "dark"
	darkModeSet, darkModeVal = true, dark
	lipgloss.SetHasDarkBackground(dark)
}

// ResolveTheme ensures a theme is configured.
// If configuredTheme is empty, it opens the interactive selector.
// Returns true if the user quit the selector without choosing.
func ResolveTheme(configuredTheme string, saveTheme func(string) error) (userQuit bool, err error) {
	if configuredTheme == "" {
		chosen, err := RunThemeSelector()
		if err != nil {
			return false, fmt.Errorf("theme selection failed: %w", err)
		}
		if chosen == "" {
			return true, nil
		}
		configuredTheme = chosen
		_ = saveTheme(configuredTheme)
	}
	InitTheme(configuredTheme)
	return false, nil
}

func IsDarkBackground() bool {
	if darkModeSet {
		return darkModeVal
	}
	return lipgloss.HasDarkBackground()
}

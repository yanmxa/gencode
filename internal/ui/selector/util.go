// Package selector provides utility functions and styles used across UI packages.
package selector

import (
	"fmt"
	"os"
	"strings"

	"github.com/yanmxa/gencode/internal/ext/mcp"
)

func FuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func CalculateBoxWidth(screenWidth int) int {
	boxWidth := screenWidth - 8
	return max(40, min(boxWidth, 60))
}

func CalculateToolBoxWidth(screenWidth int) int {
	boxWidth := screenWidth * 80 / 100
	return max(60, boxWidth)
}

// TruncateText shortens text to maxLen with ellipsis if needed.
// Returns the original text if maxLen <= 0 or if text fits within maxLen.
func TruncateText(text string, maxLen int) string {
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return text[:maxLen]
	}
	return text[:maxLen-3] + "..."
}

func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func ShortenPathForProject(path, cwd string) string {
	if strings.HasPrefix(path, cwd) {
		rel := strings.TrimPrefix(path, cwd)
		rel = strings.TrimPrefix(rel, "/")
		if rel != "" {
			return rel
		}
	}
	return ShortenPath(path)
}

// TruncateWithEllipsis is an alias for TruncateText for backward compatibility.
func TruncateWithEllipsis(s string, maxLen int) string {
	return TruncateText(s, maxLen)
}

// RenderSelectableRow renders a row with "> " or "  " prefix.
func RenderSelectableRow(line string, isSelected bool) string {
	if isSelected {
		return SelectorSelectedStyle.Render("> " + line)
	}
	return SelectorItemStyle.Render("  " + line)
}

// FormatAlignedRow formats "icon  name<padding>info" with name padded to colWidth.
func FormatAlignedRow(icon, name string, colWidth int, info string) string {
	nameWidth := len(name) // approximate; callers can use lipgloss.Width for ANSI-safe width
	pad := ""
	if nameWidth < colWidth {
		pad = strings.Repeat(" ", colWidth-nameWidth)
	}
	return fmt.Sprintf("%s  %s%s%s", icon, name, pad, info)
}

// RenderEnvVarStatus returns a styled "ENVVAR ✓" or "ENVVAR ✗" indicator.
func RenderEnvVarStatus(envVar string) string {
	if envVar == "" {
		return ""
	}
	if os.Getenv(envVar) != "" {
		return SelectorStatusReady.Render(envVar + " ✓")
	}
	return SelectorStatusNone.Render(envVar + " ✗")
}

func MCPStatusDisplay(status mcp.ServerStatus) (icon, label string) {
	switch status {
	case mcp.StatusConnected:
		return "●", "connected"
	case mcp.StatusConnecting:
		return "◌", "connecting"
	case mcp.StatusError:
		return "✗", "error"
	default:
		return "○", "disconnected"
	}
}

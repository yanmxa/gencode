package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ResultMetadata contains metadata about tool execution result
type ResultMetadata struct {
	Title      string        // Tool name
	Icon       string        // Tool icon
	Subtitle   string        // Short description (e.g., file path)
	Size       int64         // File/content size in bytes
	Duration   time.Duration // Execution duration
	LineCount  int           // Number of lines
	ItemCount  int           // Number of items (files/matches)
	StatusCode int           // HTTP status code (WebFetch)
	Truncated  bool          // Whether output was truncated
}

// RenderHeader renders the tool header box
// ┌─ Read ──────────────────────────────────────┐
// │ 📄 /path/to/file.go                         │
// │ 2.4 KB · 85 lines · 12ms                    │
// └─────────────────────────────────────────────┘
func RenderHeader(meta ResultMetadata, width int) string {
	// Title line with tool name
	title := HeaderTitleStyle.Render(meta.Title)

	// Subtitle line with icon and path/description
	subtitle := fmt.Sprintf("%s %s", meta.Icon, HeaderSubtitleStyle.Render(meta.Subtitle))

	// Meta line with size, count, duration
	metaParts := []string{}
	if meta.Size > 0 {
		metaParts = append(metaParts, FormatSize(meta.Size))
	}
	if meta.LineCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d lines", meta.LineCount))
	}
	if meta.ItemCount > 0 {
		if meta.Title == "Glob" {
			metaParts = append(metaParts, fmt.Sprintf("%d files", meta.ItemCount))
		} else if meta.Title == "Grep" {
			metaParts = append(metaParts, fmt.Sprintf("%d matches", meta.ItemCount))
		} else {
			metaParts = append(metaParts, fmt.Sprintf("%d items", meta.ItemCount))
		}
	}
	if meta.StatusCode > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d OK", meta.StatusCode))
	}
	if meta.Duration > 0 {
		metaParts = append(metaParts, FormatDuration(meta.Duration))
	}
	if meta.Truncated {
		metaParts = append(metaParts, TruncatedStyle.Render("(truncated)"))
	}
	metaLine := HeaderMetaStyle.Render(strings.Join(metaParts, " · "))

	// Build content
	content := fmt.Sprintf("%s\n%s\n%s", title, subtitle, metaLine)

	// Apply border style
	boxWidth := width
	if boxWidth <= 0 {
		boxWidth = 50
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	box := HeaderStyle.Width(boxWidth - 4).Render(content)
	return box
}

// RenderErrorHeader renders an error header box
// ┌─ Read ──────────────────────────────────────┐
// │ ❌ Error                                    │
// │ file not found: /path/to/missing.go         │
// └─────────────────────────────────────────────┘
func RenderErrorHeader(toolName, errorMsg string, width int) string {
	title := HeaderTitleStyle.Render(toolName)
	errorLine := fmt.Sprintf("%s %s", IconError, ErrorStyle.Render("Error"))
	msgLine := ErrorMsgStyle.Render(errorMsg)

	content := fmt.Sprintf("%s\n%s\n%s", title, errorLine, msgLine)

	boxWidth := width
	if boxWidth <= 0 {
		boxWidth = 50
	}
	if boxWidth > 80 {
		boxWidth = 80
	}

	// Use red border for errors
	errorBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorError).
		Padding(0, 1)

	box := errorBoxStyle.Width(boxWidth - 4).Render(content)
	return box
}

// RenderCompactHeader renders a single-line header for compact display
// 📄 Read: /path/to/file.go (2.4 KB · 85 lines · 12ms)
func RenderCompactHeader(meta ResultMetadata) string {
	metaParts := []string{}
	if meta.Size > 0 {
		metaParts = append(metaParts, FormatSize(meta.Size))
	}
	if meta.LineCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d lines", meta.LineCount))
	}
	if meta.ItemCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d items", meta.ItemCount))
	}
	if meta.Duration > 0 {
		metaParts = append(metaParts, FormatDuration(meta.Duration))
	}

	metaStr := ""
	if len(metaParts) > 0 {
		metaStr = HeaderMetaStyle.Render(fmt.Sprintf(" (%s)", strings.Join(metaParts, " · ")))
	}

	return fmt.Sprintf("%s %s: %s%s",
		meta.Icon,
		HeaderTitleStyle.Render(meta.Title),
		HeaderSubtitleStyle.Render(meta.Subtitle),
		metaStr)
}

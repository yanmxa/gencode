package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/myan/gencode/internal/tool/permission"
)

const (
	// DefaultMaxCommandLines is the default number of lines to show when collapsed
	DefaultMaxCommandLines = 20
)

// BashPreview renders a bash command preview with expand/collapse functionality
type BashPreview struct {
	bashMeta   *permission.BashMetadata
	expanded   bool
	maxVisible int
}

// NewBashPreview creates a new BashPreview instance
func NewBashPreview(meta *permission.BashMetadata) *BashPreview {
	return &BashPreview{
		bashMeta:   meta,
		expanded:   false,
		maxVisible: DefaultMaxCommandLines,
	}
}

// ToggleExpand toggles between expanded and collapsed view
func (b *BashPreview) ToggleExpand() {
	b.expanded = !b.expanded
}

// IsExpanded returns whether the preview is expanded
func (b *BashPreview) IsExpanded() bool {
	return b.expanded
}

// SetMaxVisible sets the maximum visible lines when collapsed
func (b *BashPreview) SetMaxVisible(n int) {
	b.maxVisible = n
}

// Styles for bash command rendering
var (
	bashCommandStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D1D5DB")) // gray-300

	bashLineNoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4B5563")). // dark gray
			Width(4).
			Align(lipgloss.Right)

	bashBgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")) // amber

	bashDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")). // gray-400
			Italic(true)
)

// Render renders the bash command preview
func (b *BashPreview) Render(width int) string {
	if b.bashMeta == nil || b.bashMeta.Command == "" {
		return bashCommandStyle.Render("  (no command)")
	}

	var sb strings.Builder

	// Show description if provided
	if b.bashMeta.Description != "" {
		sb.WriteString(" ")
		sb.WriteString(bashDescStyle.Render(b.bashMeta.Description))
		sb.WriteString("\n")
	}

	// Show background indicator
	if b.bashMeta.RunBackground {
		sb.WriteString(" ")
		sb.WriteString(bashBgStyle.Render("[background]"))
		sb.WriteString("\n")
	}

	lines := strings.Split(b.bashMeta.Command, "\n")
	showCount := len(lines)
	truncated := false

	if !b.expanded && showCount > b.maxVisible {
		showCount = b.maxVisible
		truncated = true
	}

	// Render command lines with line numbers
	for i := 0; i < showCount; i++ {
		lineNo := fmt.Sprintf("%3d ", i+1)
		sb.WriteString(bashLineNoStyle.Render(lineNo))
		sb.WriteString(bashLineNoStyle.Render("│"))
		sb.WriteString(bashCommandStyle.Render(" " + lines[i]))
		sb.WriteString("\n")
	}

	// Show truncation message
	if truncated {
		remaining := len(lines) - b.maxVisible
		msg := fmt.Sprintf("... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(diffMoreStyle.Render(msg))
		sb.WriteString("\n")
	}

	return sb.String()
}

// NeedsExpand returns true if the command has more lines than the default visible count
func (b *BashPreview) NeedsExpand() bool {
	if b.bashMeta == nil {
		return false
	}
	return b.bashMeta.LineCount > b.maxVisible
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool/permission"
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

// Bash command rendering styles - use functions to get current theme dynamically
func getBashCommandStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Text)
}

func getBashBgStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Warning)
}

func getBashDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.TextDim).Italic(true)
}

// Render renders the bash command preview with simplified format (3-space indent, no line numbers)
func (b *BashPreview) Render(width int) string {
	if b.bashMeta == nil || b.bashMeta.Command == "" {
		return getBashCommandStyle().Render("   (no command)")
	}

	var sb strings.Builder

	// Show background indicator first if needed
	if b.bashMeta.RunBackground {
		sb.WriteString("   ")
		sb.WriteString(getBashBgStyle().Render("[background]"))
		sb.WriteString("\n")
	}

	lines := strings.Split(b.bashMeta.Command, "\n")
	showCount := len(lines)
	truncated := false

	if !b.expanded && showCount > b.maxVisible {
		showCount = b.maxVisible
		truncated = true
	}

	// Render command lines with 3-space indent (no line numbers)
	for i := 0; i < showCount; i++ {
		sb.WriteString("   ")
		sb.WriteString(getBashCommandStyle().Render(lines[i]))
		sb.WriteString("\n")
	}

	// Show description after command (3-space indent)
	if b.bashMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(getBashDescStyle().Render(b.bashMeta.Description))
		sb.WriteString("\n")
	}

	// Show truncation message
	if truncated {
		remaining := len(lines) - b.maxVisible
		msg := fmt.Sprintf("   ... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(getDiffMoreStyle().Render(msg))
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

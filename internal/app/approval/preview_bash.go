package approval

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/app/theme"
)

const (
	// defaultMaxCommandLines is the default number of lines to show when collapsed
	defaultMaxCommandLines = 20
)

// bashPreview renders a bash command preview with expand/collapse functionality
type bashPreview struct {
	bashMeta   *perm.BashMetadata
	expanded   bool
	maxVisible int
}

// newBashPreview creates a new bashPreview instance
func newBashPreview(meta *perm.BashMetadata) *bashPreview {
	return &bashPreview{
		bashMeta:   meta,
		expanded:   false,
		maxVisible: defaultMaxCommandLines,
	}
}

// toggleExpand toggles between expanded and collapsed view
func (b *bashPreview) toggleExpand() {
	b.expanded = !b.expanded
}

// isExpanded returns whether the preview is expanded
func (b *bashPreview) isExpanded() bool {
	return b.expanded
}

// setMaxVisible sets the maximum visible lines when collapsed
func (b *bashPreview) setMaxVisible(n int) {
	b.maxVisible = n
}

// Bash command rendering styles - use functions to get current theme dynamically
func getBashCommandStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text)
}

func getBashBgStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
}

func getBashDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim).Italic(true)
}

// Render renders the bash command preview with simplified format (3-space indent, no line numbers)
func (b *bashPreview) render(width int) string {
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

	const indent = 3
	contentWidth := width - indent
	if contentWidth < 8 {
		contentWidth = 8
	}

	// Render command lines with 3-space indent (no line numbers)
	for i := 0; i < showCount; i++ {
		sb.WriteString("   ")
		sb.WriteString(getBashCommandStyle().Render(truncateContent(lines[i], contentWidth)))
		sb.WriteString("\n")
	}

	// Show description after command (3-space indent)
	if b.bashMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(getBashDescStyle().Render(truncateContent(b.bashMeta.Description, contentWidth)))
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

// needsExpand returns true if the command has more lines than the default visible count
func (b *bashPreview) needsExpand() bool {
	if b.bashMeta == nil {
		return false
	}
	return b.bashMeta.LineCount > b.maxVisible
}

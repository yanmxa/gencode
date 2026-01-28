package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/yanmxa/gencode/internal/tool/permission"
)

const (
	// DefaultMaxVisibleLines is the default number of lines to show when collapsed
	DefaultMaxVisibleLines = 20
)

// DiffPreview renders a diff preview with expand/collapse functionality
type DiffPreview struct {
	diffMeta   *permission.DiffMetadata
	expanded   bool
	maxVisible int
}

// NewDiffPreview creates a new DiffPreview instance
func NewDiffPreview(diffMeta *permission.DiffMetadata) *DiffPreview {
	return &DiffPreview{
		diffMeta:   diffMeta,
		expanded:   false,
		maxVisible: DefaultMaxVisibleLines,
	}
}

// ToggleExpand toggles between expanded and collapsed view
func (d *DiffPreview) ToggleExpand() {
	d.expanded = !d.expanded
}

// IsExpanded returns whether the preview is expanded
func (d *DiffPreview) IsExpanded() bool {
	return d.expanded
}

// SetMaxVisible sets the maximum visible lines when collapsed
func (d *DiffPreview) SetMaxVisible(n int) {
	d.maxVisible = n
}

// Diff rendering styles (initialized dynamically based on theme)
var (
	diffAddedStyle   lipgloss.Style
	diffRemovedStyle lipgloss.Style
	diffContextStyle lipgloss.Style
	diffLineNoStyle  lipgloss.Style
	diffSummaryStyle lipgloss.Style
	diffMoreStyle    lipgloss.Style
)

func init() {
	// Initialize diff styles based on current theme
	diffAddedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Success)

	diffRemovedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Error)

	diffContextStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	diffLineNoStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDisabled).
		Width(5).
		Align(lipgloss.Right)

	diffSummaryStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim)

	diffMoreStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted).
		Italic(true)
}

// Render renders the diff preview
func (d *DiffPreview) Render(width int) string {
	if d.diffMeta == nil || len(d.diffMeta.Lines) == 0 {
		return diffContextStyle.Render("  (no changes)")
	}

	// For new files or preview mode, use simple single-column format
	if d.diffMeta.IsNewFile || d.diffMeta.PreviewMode {
		return d.renderNewFilePreview(width)
	}

	// For edits, use side-by-side format
	return d.renderSideBySide(width)
}

// renderNewFilePreview renders a new file in simple single-column format
func (d *DiffPreview) renderNewFilePreview(width int) string {
	var sb strings.Builder

	lines := d.diffMeta.Lines
	showCount := len(lines)
	truncated := false

	if !d.expanded && showCount > d.maxVisible {
		showCount = d.maxVisible
		truncated = true
	}

	for i := 0; i < showCount; i++ {
		line := lines[i]
		// Skip hunk headers and metadata for new files
		if line.Type == permission.DiffLineHunk || line.Type == permission.DiffLineMetadata {
			continue
		}
		lineNo := fmt.Sprintf("%4d", line.NewLineNo)
		sb.WriteString(diffLineNoStyle.Render(lineNo))
		sb.WriteString(diffLineNoStyle.Render(" │ "))
		sb.WriteString(diffAddedStyle.Render(line.Content))
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(lines) - d.maxVisible
		msg := fmt.Sprintf("... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(diffMoreStyle.Render(msg))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderSideBySide renders edits in side-by-side format:
// Left side: removed lines (old content)
// Right side: added lines (new content)
// Arrow (→) connects them
func (d *DiffPreview) renderSideBySide(width int) string {
	var sb strings.Builder

	// Summary line: +5 -3 lines
	summary := fmt.Sprintf("+%d -%d lines", d.diffMeta.AddedCount, d.diffMeta.RemovedCount)
	sb.WriteString(diffSummaryStyle.Render(summary))
	sb.WriteString("\n")

	// Collect removed and added lines
	var removed, added []permission.DiffLine
	for _, line := range d.diffMeta.Lines {
		switch line.Type {
		case permission.DiffLineRemoved:
			removed = append(removed, line)
		case permission.DiffLineAdded:
			added = append(added, line)
		}
		// Skip context, hunk, and metadata lines for side-by-side view
	}

	// Calculate column width (account for line numbers, borders, arrow)
	// Format: "  1 │ content     →  1 │ content"
	// Line number column: 4 chars + " │ " (3 chars) = 7 chars per side
	// Arrow: " → " = 3 chars
	colWidth := (width - 7 - 3 - 7) / 2
	if colWidth < 10 {
		colWidth = 10
	}

	// Pair and render rows
	maxRows := len(removed)
	if len(added) > maxRows {
		maxRows = len(added)
	}

	// Apply truncation
	showRows := maxRows
	truncated := false
	if !d.expanded && showRows > d.maxVisible {
		showRows = d.maxVisible
		truncated = true
	}

	for i := 0; i < showRows; i++ {
		// Left side: removed content (red)
		if i < len(removed) {
			lineNo := fmt.Sprintf("%4d", removed[i].OldLineNo)
			sb.WriteString(diffLineNoStyle.Render(lineNo))
			sb.WriteString(diffLineNoStyle.Render(" │ "))
			content := truncateOrPad(removed[i].Content, colWidth)
			sb.WriteString(diffRemovedStyle.Render(content))
		} else {
			// Empty placeholder for left side
			sb.WriteString(strings.Repeat(" ", 7+colWidth))
		}

		// Arrow separator
		sb.WriteString(diffContextStyle.Render(" → "))

		// Right side: added content (green)
		if i < len(added) {
			lineNo := fmt.Sprintf("%4d", added[i].NewLineNo)
			sb.WriteString(diffLineNoStyle.Render(lineNo))
			sb.WriteString(diffLineNoStyle.Render(" │ "))
			sb.WriteString(diffAddedStyle.Render(added[i].Content))
		}

		sb.WriteString("\n")
	}

	if truncated {
		remaining := maxRows - d.maxVisible
		msg := fmt.Sprintf("... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(diffMoreStyle.Render(msg))
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncateOrPad truncates content if too long, or pads with spaces if too short
// Uses runewidth for proper CJK character handling (each CJK char = 2 terminal columns)
func truncateOrPad(content string, width int) string {
	displayWidth := runewidth.StringWidth(content)
	if displayWidth > width {
		// Truncate with ellipsis
		if width > 3 {
			return runewidth.Truncate(content, width-3, "...")
		}
		return runewidth.Truncate(content, width, "")
	}
	// Pad with spaces to reach target width
	padding := width - displayWidth
	return content + strings.Repeat(" ", padding)
}

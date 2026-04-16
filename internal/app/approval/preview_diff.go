package approval

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

const (
	// defaultMaxVisibleLines is the default number of lines to show when collapsed
	defaultMaxVisibleLines = 20
)

// diffPreview renders a diff preview with expand/collapse functionality
type diffPreview struct {
	diffMeta   *perm.DiffMetadata
	filePath   string
	expanded   bool
	maxVisible int
}

// NewdiffPreview creates a new diffPreview instance
func newDiffPreview(diffMeta *perm.DiffMetadata, filePath string) *diffPreview {
	return &diffPreview{
		diffMeta:   diffMeta,
		filePath:   filePath,
		expanded:   false,
		maxVisible: defaultMaxVisibleLines,
	}
}

// toggleExpand toggles between expanded and collapsed view
func (d *diffPreview) toggleExpand() {
	d.expanded = !d.expanded
}

// isExpanded returns whether the preview is expanded
func (d *diffPreview) isExpanded() bool {
	return d.expanded
}

// setMaxVisible sets the maximum visible lines when collapsed
func (d *diffPreview) setMaxVisible(n int) {
	d.maxVisible = n
}

// Diff rendering styles - use functions to get current theme dynamically
func getDiffAddedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)
}

func getDiffRemovedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Error)
}

func getDiffAddedBgStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success).Background(theme.CurrentTheme.SuccessBg)
}

func getDiffRemovedBgStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Error).Background(theme.CurrentTheme.ErrorBg)
}

func getDiffContextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
}

func getDiffLineNoStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDisabled).
		Width(5).
		Align(lipgloss.Right)
}

func getDiffMoreStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Italic(true)
}

func getDiffHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
}

// render renders the diff preview
func (d *diffPreview) render(width int) string {
	if d.diffMeta == nil || len(d.diffMeta.Lines) == 0 {
		return getDiffContextStyle().Render("  (no changes)")
	}

	// For new files or preview mode, use simple single-column format
	if d.diffMeta.IsNewFile || d.diffMeta.PreviewMode {
		return d.renderNewFilePreview(width)
	}

	// For edits, use unified single-panel format
	return d.renderUnifiedDiff(width)
}

// renderFileHeader renders a file path header with summary info
func (d *diffPreview) renderFileHeader(label string) string {
	name := d.filePath
	if name != "" {
		name = filepath.Base(name)
	}
	if name == "" {
		name = "file"
	}
	return getDiffHeaderStyle().Render(fmt.Sprintf(" %s  %s", name, label))
}

// renderNewFilePreview renders a new file in simple single-column format
func (d *diffPreview) renderNewFilePreview(width int) string {
	var sb strings.Builder

	// File header
	sb.WriteString(d.renderFileHeader("(new file)"))
	sb.WriteString("\n")
	sep := strings.Repeat("─", width)
	sb.WriteString(getDiffContextStyle().Render(sep))
	sb.WriteString("\n")

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
		if line.Type == perm.DiffLineHunk || line.Type == perm.DiffLineMetadata {
			continue
		}
		lineNo := fmt.Sprintf("%4d", line.NewLineNo)
		sb.WriteString(getDiffLineNoStyle().Render(lineNo))
		sb.WriteString(getDiffContextStyle().Render(" │ "))
		sb.WriteString(getDiffAddedStyle().Render(line.Content))
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(lines) - d.maxVisible
		msg := fmt.Sprintf("... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(getDiffMoreStyle().Render(msg))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderUnifiedDiff renders edits in a unified single-panel format.
// Removed lines shown with " - " indicator and red/error background.
// Added lines shown with " + " indicator and green/success background.
// Context lines shown with dim text, no background.
func (d *diffPreview) renderUnifiedDiff(width int) string {
	var sb strings.Builder

	// File header with summary
	added := getDiffAddedStyle().Render(fmt.Sprintf("+%d", d.diffMeta.AddedCount))
	removed := getDiffRemovedStyle().Render(fmt.Sprintf("-%d", d.diffMeta.RemovedCount))
	sb.WriteString(d.renderFileHeader(fmt.Sprintf("%s %s", added, removed)))
	sb.WriteString("\n")

	// Layout: "NNNN - content" = 4 (lineNo) + 3 (indicator) + content
	const prefix = 7
	contentWidth := width - prefix
	if contentWidth < 8 {
		contentWidth = 8
	}

	lines := d.diffMeta.Lines
	showCount := len(lines)
	truncated := false
	if !d.expanded && showCount > d.maxVisible {
		showCount = d.maxVisible
		truncated = true
	}

	removedBgStyle := getDiffRemovedBgStyle()
	addedBgStyle := getDiffAddedBgStyle()
	contextStyle := getDiffContextStyle()

	for i := 0; i < showCount; i++ {
		line := lines[i]

		switch line.Type {
		case perm.DiffLineHunk, perm.DiffLineMetadata:
			// Skip hunk headers and metadata entirely

		case perm.DiffLineContext:
			no := fmt.Sprintf("%4d", line.OldLineNo)
			content := truncateContent(line.Content, contentWidth)
			sb.WriteString(contextStyle.Render(no + "   " + content))
			sb.WriteString("\n")

		case perm.DiffLineRemoved:
			no := fmt.Sprintf("%4d", line.OldLineNo)
			content := truncateOrPad(truncateContent(line.Content, contentWidth), contentWidth)
			sb.WriteString(removedBgStyle.Render(truncateOrPad(no+" - "+content, width)))
			sb.WriteString("\n")

		case perm.DiffLineAdded:
			no := fmt.Sprintf("%4d", line.NewLineNo)
			content := truncateOrPad(truncateContent(line.Content, contentWidth), contentWidth)
			sb.WriteString(addedBgStyle.Render(truncateOrPad(no+" + "+content, width)))
			sb.WriteString("\n")
		}
	}

	if truncated {
		remaining := len(lines) - d.maxVisible
		msg := fmt.Sprintf("... %d more lines (Ctrl+O to expand)", remaining)
		sb.WriteString(getDiffMoreStyle().Render(msg))
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncateContent truncates content if too long
func truncateContent(content string, width int) string {
	displayWidth := runewidth.StringWidth(content)
	if displayWidth > width {
		if width > 3 {
			return runewidth.Truncate(content, width-3, "...")
		}
		return runewidth.Truncate(content, width, "")
	}
	return content
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

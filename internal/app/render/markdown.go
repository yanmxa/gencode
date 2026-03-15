// Markdown renderer using glamour for styled terminal output.
// Tables are rendered separately with lipgloss/table for full border control,
// since glamour hardcodes outer borders off (ansi/table.go setBorders).
package render

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

// MDRenderer renders markdown content to styled terminal output using glamour.
type MDRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// NewMDRenderer creates a new markdown renderer with the given width.
func NewMDRenderer(width int) *MDRenderer {
	w := max(width-4, MinWrapWidth)

	// Pick base style based on terminal background
	var style ansi.StyleConfig
	if lipgloss.HasDarkBackground() {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}
	customizeStyle(&style, w)

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(w),
		glamour.WithChromaFormatter("terminal256"),
	)
	if err != nil {
		r, _ = glamour.NewTermRenderer(glamour.WithAutoStyle())
	}
	return &MDRenderer{renderer: r, width: w}
}

// Render parses markdown source and returns styled terminal output.
// Tables are extracted and rendered with lipgloss/table for full border control,
// everything else (including code blocks) goes through glamour natively.
func (r *MDRenderer) Render(content string) (string, error) {
	segments := splitTables(content)

	var parts []string
	for _, seg := range segments {
		switch seg.kind {
		case segTable:
			parts = append(parts, r.renderTable(seg.content))
		default:
			rendered, err := r.renderer.Render(seg.content)
			if err != nil {
				parts = append(parts, seg.content)
			} else {
				parts = append(parts, strings.TrimRight(rendered, "\n"))
			}
		}
	}

	return strings.TrimRight(strings.Join(parts, ""), "\n"), nil
}

// segmentKind identifies what type of markdown block a segment contains.
type segmentKind int

const (
	segPlain segmentKind = iota
	segTable
)

// segment represents a piece of markdown content.
type segment struct {
	content string
	kind    segmentKind
}

// splitTables splits markdown content into table and non-table segments.
// Tables are rendered separately with lipgloss/table for full border control.
func splitTables(content string) []segment {
	lines := strings.Split(content, "\n")
	var segments []segment
	var plain []string

	i := 0
	for i < len(lines) {
		if isTableLine(lines[i]) {
			tableEnd := findTableEnd(lines, i)
			if tableEnd > i+1 && hasTableSeparator(lines, i, tableEnd) {
				if len(plain) > 0 {
					segments = append(segments, segment{content: strings.Join(plain, "\n"), kind: segPlain})
					plain = nil
				}
				tableLines := strings.Join(lines[i:tableEnd], "\n")
				segments = append(segments, segment{content: tableLines, kind: segTable})
				i = tableEnd
				continue
			}
		}
		plain = append(plain, lines[i])
		i++
	}

	if len(plain) > 0 {
		segments = append(segments, segment{content: strings.Join(plain, "\n"), kind: segPlain})
	}
	return segments
}

// isTableLine checks if a line looks like a markdown table line (starts with |).
func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed[1:], "|")
}

// findTableEnd finds the end index (exclusive) of consecutive table lines.
func findTableEnd(lines []string, start int) int {
	i := start
	for i < len(lines) && isTableLine(lines[i]) {
		i++
	}
	return i
}

// hasTableSeparator checks if there's a separator line (|---|) in the range.
func hasTableSeparator(lines []string, start, end int) bool {
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		// A separator line contains |, -, and optionally : for alignment
		cleaned := strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(trimmed)
		if cleaned == "" && strings.Contains(trimmed, "-") {
			return true
		}
	}
	return false
}

// renderTable renders a markdown table using lipgloss/table with full borders.
func (r *MDRenderer) renderTable(content string) string {
	headers, rows := parseMarkdownTable(content)
	if len(headers) == 0 {
		return content
	}

	borderColor := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Separator)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.CurrentTheme.Text)

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderColor).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		BorderHeader(true).
		BorderColumn(true).
		BorderRow(false).
		Width(r.width).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text)
		})

	return "\n" + t.String() + "\n"
}

// parseMarkdownTable extracts headers and rows from a markdown table string.
func parseMarkdownTable(content string) ([]string, [][]string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	var headers []string
	var rows [][]string
	headerParsed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check if this is a separator line (|---|---|)
		cleaned := strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(trimmed)
		if cleaned == "" && strings.Contains(trimmed, "-") {
			headerParsed = true
			continue
		}

		cells := parseTableRow(trimmed)
		if !headerParsed {
			headers = cells
		} else {
			rows = append(rows, cells)
		}
	}

	return headers, rows
}

// parseTableRow splits a markdown table row into cells and renders inline markdown.
func parseTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")

	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = renderInlineMarkdown(strings.TrimSpace(p))
	}
	return cells
}

// renderInlineMarkdown renders inline markdown elements: `code`, **bold**, *italic*.
func renderInlineMarkdown(text string) string {
	var result strings.Builder
	i := 0
	for i < len(text) {
		// Inline code: `...`
		if text[i] == '`' {
			end := strings.Index(text[i+1:], "`")
			if end != -1 {
				code := text[i+1 : i+1+end]
				codeStyle := lipgloss.NewStyle().Background(theme.CurrentTheme.Background)
				result.WriteString(codeStyle.Render(code))
				i += end + 2
				continue
			}
		}
		// Bold: **...**
		if i+1 < len(text) && text[i] == '*' && text[i+1] == '*' {
			end := strings.Index(text[i+2:], "**")
			if end != -1 {
				bold := text[i+2 : i+2+end]
				boldStyle := lipgloss.NewStyle().Bold(true)
				result.WriteString(boldStyle.Render(bold))
				i += end + 4
				continue
			}
		}
		// Italic: *...*
		if text[i] == '*' {
			end := strings.Index(text[i+1:], "*")
			if end != -1 {
				italic := text[i+1 : i+1+end]
				italicStyle := lipgloss.NewStyle().Italic(true)
				result.WriteString(italicStyle.Render(italic))
				i += end + 2
				continue
			}
		}
		result.WriteByte(text[i])
		i++
	}
	return result.String()
}

// customizeStyle adjusts glamour's default style for a clean, unified look.
// Uses only 3 accent colors: blue (keywords/headings), green (strings/functions), muted (comments).
func customizeStyle(s *ansi.StyleConfig, width int) {
	blue := string(theme.CurrentTheme.Primary)
	muted := string(theme.CurrentTheme.Muted)

	// Headings: blue, bold, no prefix markers
	s.H1.Prefix = ""
	s.H1.Suffix = ""
	s.H1.Color = &blue
	s.H1.BackgroundColor = nil
	s.H1.Bold = boolPtr(true)
	s.H2.Prefix = ""
	s.H2.Color = &blue
	s.H2.Bold = boolPtr(true)
	s.H3.Prefix = ""
	s.H3.Color = &blue
	s.H3.Bold = boolPtr(true)
	s.H4.Prefix = ""
	s.H5.Prefix = ""
	s.H6.Prefix = ""

	// Horizontal rule: full-width thin line
	hr := strings.Repeat("─", width)
	s.HorizontalRule.Format = "\n" + hr + "\n"
	s.HorizontalRule.Color = &muted

	// Inline code: subtle background only
	bg := string(theme.CurrentTheme.Background)
	s.Code.StylePrimitive.BackgroundColor = &bg
	s.Code.StylePrimitive.Prefix = ""
	s.Code.StylePrimitive.Suffix = ""
	s.Code.StylePrimitive.Color = nil

	// Reduce document margin for tighter layout
	margin := uint(0)
	s.Document.Margin = &margin
}

func boolPtr(b bool) *bool { return &b }

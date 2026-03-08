// Lightweight markdown renderer using goldmark AST + chroma syntax highlighting + lipgloss styling.
// Replaces glamour with a minimal renderer that handles core markdown elements.
package render

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromaStyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/yanmxa/gencode/internal/ui/theme"
)

// MDRenderer renders markdown content to styled terminal output.
type MDRenderer struct {
	width int
	dark  bool
}

// NewMDRenderer creates a new lightweight markdown renderer.
func NewMDRenderer(width int) *MDRenderer {
	return &MDRenderer{
		width: max(width-4, MinWrapWidth),
		dark:  lipgloss.HasDarkBackground(),
	}
}

// Render parses markdown source and returns styled terminal output.
func (r *MDRenderer) Render(content string) (string, error) {
	source := []byte(content)
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
	)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var buf strings.Builder
	r.renderNode(&buf, doc, source, 0)
	return buf.String(), nil
}

// renderNode walks the AST and renders each node.
func (r *MDRenderer) renderNode(buf *strings.Builder, n ast.Node, source []byte, depth int) {
	switch node := n.(type) {
	case *ast.Document:
		r.renderChildren(buf, node, source, depth)

	case *ast.Heading:
		r.renderHeading(buf, node, source)

	case *ast.Paragraph:
		r.renderParagraph(buf, node, source, depth)

	case *ast.FencedCodeBlock:
		r.renderFencedCode(buf, node, source)

	case *ast.CodeBlock:
		r.renderFencedCode(buf, node, source)

	case *ast.List:
		r.renderList(buf, node, source, depth)

	case *ast.TextBlock:
		content := r.renderInlineChildren(node, source)
		if content != "" {
			buf.WriteString(content)
			buf.WriteByte('\n')
		}

	case *ast.ThematicBreak:
		r.renderThematicBreak(buf)

	case *ast.Blockquote:
		r.renderBlockquote(buf, node, source, depth)

	case *east.Table:
		r.renderTable(buf, node, source)

	default:
		// For unknown block nodes, render children
		if n.HasChildren() {
			r.renderChildren(buf, n, source, depth)
		}
	}
}

// renderChildren renders all child nodes, adding blank lines only before headings and code blocks.
func (r *MDRenderer) renderChildren(buf *strings.Builder, n ast.Node, source []byte, depth int) {
	hasPrev := false
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if hasPrev && needsBlankLineBefore(child) {
			buf.WriteByte('\n')
		}
		r.renderNode(buf, child, source, depth)
		hasPrev = true
	}
}

// needsBlankLineBefore returns true for nodes that need a blank line above for visual separation.
func needsBlankLineBefore(n ast.Node) bool {
	switch n.(type) {
	case *ast.Heading, *ast.FencedCodeBlock, *ast.CodeBlock, *east.Table:
		return true
	}
	return false
}

// renderHeading renders a heading with bold + color based on level.
func (r *MDRenderer) renderHeading(buf *strings.Builder, node *ast.Heading, source []byte) {
	var color lipgloss.Color
	switch node.Level {
	case 1:
		color = theme.CurrentTheme.Primary
	case 2:
		color = theme.CurrentTheme.Accent
	default:
		color = theme.CurrentTheme.Muted
	}

	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	content := r.renderInlineChildren(node, source)
	buf.WriteString(style.Render(content))
	buf.WriteByte('\n')
}

// renderParagraph renders a paragraph with word wrapping.
func (r *MDRenderer) renderParagraph(buf *strings.Builder, node *ast.Paragraph, source []byte, _ int) {
	content := r.renderInlineChildren(node, source)
	if content == "" {
		return
	}

	wrapped := r.wordWrap(content, r.width)
	buf.WriteString(wrapped)
	buf.WriteByte('\n')
}

// renderFencedCode renders a code block with syntax highlighting.
func (r *MDRenderer) renderFencedCode(buf *strings.Builder, node ast.Node, source []byte) {
	// Extract language
	var lang string
	if fenced, ok := node.(*ast.FencedCodeBlock); ok {
		lang = string(fenced.Language(source))
	}

	// Extract code content
	var code strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		code.Write(line.Value(source))
	}
	codeStr := code.String()

	// Try syntax highlighting with chroma
	highlighted := r.highlightCode(codeStr, lang)
	if highlighted != "" {
		// Indent each line by 2 spaces
		for line := range strings.SplitSeq(highlighted, "\n") {
			if line != "" {
				buf.WriteString("  ")
				buf.WriteString(line)
			}
			buf.WriteByte('\n')
		}
	} else {
		// Fallback: plain indented code
		codeStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text)
		for line := range strings.SplitSeq(codeStr, "\n") {
			buf.WriteString("  ")
			buf.WriteString(codeStyle.Render(line))
			buf.WriteByte('\n')
		}
	}
}

// highlightCode uses chroma to syntax-highlight code.
func (r *MDRenderer) highlightCode(code, lang string) string {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	var styleName string
	if r.dark {
		styleName = "monokai"
	} else {
		styleName = "monokailight"
	}
	style := chromaStyles.Get(styleName)

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return ""
	}

	return strings.TrimRight(buf.String(), "\n")
}

// renderList renders ordered and unordered lists.
func (r *MDRenderer) renderList(buf *strings.Builder, node *ast.List, source []byte, depth int) {
	idx := 1
	if node.Start > 0 {
		idx = node.Start
	}

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		listItem, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}

		indent := strings.Repeat("  ", depth)

		// Determine bullet/number
		var marker string
		if node.IsOrdered() {
			marker = fmt.Sprintf("%d. ", idx)
			idx++
		} else {
			marker = "• "
		}

		markerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
		buf.WriteString(indent)
		buf.WriteString(markerStyle.Render(marker))

		// Calculate available width for list item content
		contentIndent := indent + strings.Repeat(" ", utf8.RuneCountInString(marker))
		contentWidth := r.width - utf8.RuneCountInString(indent) - utf8.RuneCountInString(marker)

		// Render list item content
		first := true
		for itemChild := listItem.FirstChild(); itemChild != nil; itemChild = itemChild.NextSibling() {
			switch ic := itemChild.(type) {
			case *ast.Paragraph:
				content := r.renderInlineChildren(ic, source)
				if !first {
					buf.WriteString(contentIndent)
				}
				wrapped := r.wordWrap(content, contentWidth)
				// Indent continuation lines
				wrapped = strings.ReplaceAll(wrapped, "\n", "\n"+contentIndent)
				buf.WriteString(wrapped)
			case *ast.TextBlock:
				content := r.renderInlineChildren(ic, source)
				if !first {
					buf.WriteString(contentIndent)
				}
				buf.WriteString(content)
			case *ast.List:
				if first {
					buf.WriteByte('\n')
				}
				r.renderList(buf, ic, source, depth+1)
				continue
			default:
				if itemChild.HasChildren() {
					r.renderChildren(buf, itemChild, source, depth+1)
				}
			}
			first = false
		}
		buf.WriteByte('\n')
	}
}

// renderTable renders a table with aligned columns.
func (r *MDRenderer) renderTable(buf *strings.Builder, node *east.Table, source []byte) {
	// Collect all rows (header + body rows are direct children of Table)
	var rows [][]string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch section := child.(type) {
		case *east.TableHeader:
			rows = append(rows, r.collectTableCells(section, source))
		case *east.TableRow:
			rows = append(rows, r.collectTableCells(section, source))
		}
	}

	if len(rows) == 0 {
		return
	}

	// Calculate column widths
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	colWidths := make([]int, numCols)
	for _, row := range rows {
		for i, cell := range row {
			w := lipgloss.Width(cell)
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Separator)

	// Helper to render a horizontal border line with box-drawing corners
	renderBorder := func(left, mid, right string) {
		var line strings.Builder
		line.WriteString(left)
		for j := 0; j < numCols; j++ {
			if j > 0 {
				line.WriteString(mid)
			}
			line.WriteString(strings.Repeat("─", colWidths[j]+2))
		}
		line.WriteString(right)
		buf.WriteString(dimStyle.Render(line.String()))
		buf.WriteByte('\n')
	}

	// Top border: ┌──────┬─────┐
	renderBorder("┌", "┬", "┐")

	for i, row := range rows {
		buf.WriteString(dimStyle.Render("│") + " ")
		for j := 0; j < numCols; j++ {
			if j > 0 {
				buf.WriteString(" " + dimStyle.Render("│") + " ")
			}
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			pad := strings.Repeat(" ", colWidths[j]-lipgloss.Width(cell))
			if i == 0 {
				buf.WriteString("\x1b[1m" + cell + "\x1b[22m" + pad)
			} else {
				buf.WriteString(cell + pad)
			}
		}
		buf.WriteString(" " + dimStyle.Render("│"))
		buf.WriteByte('\n')

		// Separator after each row except the last
		if i < len(rows)-1 {
			renderBorder("├", "┼", "┤")
		}
	}

	// Bottom border: └──────┴─────┘
	renderBorder("└", "┴", "┘")
}

// collectTableCells extracts cell text from a table row or header.
func (r *MDRenderer) collectTableCells(node ast.Node, source []byte) []string {
	var cells []string
	for cell := node.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if tc, ok := cell.(*east.TableCell); ok {
			cells = append(cells, r.renderInlineChildren(tc, source))
		}
	}
	return cells
}

// renderBlockquote renders a blockquote with a left border indicator.
func (r *MDRenderer) renderBlockquote(buf *strings.Builder, node *ast.Blockquote, source []byte, depth int) {
	// Render children into a temporary buffer
	var inner strings.Builder
	r.renderChildren(&inner, node, source, depth)

	quoteStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	for line := range strings.SplitSeq(inner.String(), "\n") {
		if line != "" {
			buf.WriteString(quoteStyle.Render("│ " + line))
		}
		buf.WriteByte('\n')
	}
}

// renderThematicBreak renders a horizontal rule.
func (r *MDRenderer) renderThematicBreak(buf *strings.Builder) {
	width := min(r.width, 40)
	style := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Separator)
	buf.WriteString(style.Render(strings.Repeat("─", width)))
	buf.WriteByte('\n')
}

// renderInlineChildren renders all inline children of a node into a string.
func (r *MDRenderer) renderInlineChildren(n ast.Node, source []byte) string {
	var buf strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		r.renderInline(&buf, child, source)
	}
	return buf.String()
}

// renderInline renders a single inline node.
func (r *MDRenderer) renderInline(buf *strings.Builder, n ast.Node, source []byte) {
	switch node := n.(type) {
	case *ast.Text:
		buf.Write(node.Segment.Value(source))
		if node.SoftLineBreak() {
			buf.WriteByte(' ')
		}
		if node.HardLineBreak() {
			buf.WriteByte('\n')
		}

	case *ast.String:
		buf.Write(node.Value)

	case *ast.CodeSpan:
		code := r.extractTextContent(node, source)
		style := lipgloss.NewStyle().
			Background(theme.CurrentTheme.Background).
			Foreground(theme.CurrentTheme.Accent)
		buf.WriteString(style.Render(code))

	case *ast.Emphasis:
		content := r.renderInlineChildren(node, source)
		if node.Level == 2 {
			// Bold
			buf.WriteString("\x1b[1m" + content + "\x1b[22m")
		} else {
			// Italic
			buf.WriteString("\x1b[3m" + content + "\x1b[23m")
		}

	case *ast.Link:
		linkText := r.renderInlineChildren(node, source)
		url := string(node.Destination)
		// Render as OSC 8 clickable hyperlink: blue + dashed underline, no URL shown
		linkStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary)
		styled := "\x1b[4:5m" + linkStyle.Render(linkText) + "\x1b[4:0m"
		// Wrap with OSC 8 escape sequence for terminal hyperlink
		buf.WriteString("\x1b]8;;" + url + "\x1b\\" + styled + "\x1b]8;;\x1b\\")

	case *ast.AutoLink:
		url := string(node.URL(source))
		linkStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary)
		styled := "\x1b[4:5m" + linkStyle.Render(url) + "\x1b[4:0m"
		buf.WriteString("\x1b]8;;" + url + "\x1b\\" + styled + "\x1b]8;;\x1b\\")

	case *ast.Image:
		alt := r.renderInlineChildren(node, source)
		if alt != "" {
			buf.WriteString("[" + alt + "]")
		} else {
			buf.WriteString("[image]")
		}

	case *ast.RawHTML:
		segs := node.Segments
		for i := 0; i < segs.Len(); i++ {
			seg := segs.At(i)
			buf.Write(seg.Value(source))
		}

	default:
		// Render children or raw text
		if n.HasChildren() {
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				r.renderInline(buf, child, source)
			}
		}
	}
}

// extractTextContent extracts all text content from an inline node's children.
func (r *MDRenderer) extractTextContent(n ast.Node, source []byte) string {
	var buf strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
	return buf.String()
}

// wordWrap wraps text to the given width, preserving existing line breaks.
func (r *MDRenderer) wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		r.wrapLine(&result, line, width)
	}
	return result.String()
}

// wrapLine wraps a single line to the given width.
func (r *MDRenderer) wrapLine(buf *strings.Builder, line string, width int) {
	if len(line) <= width {
		buf.WriteString(line)
		return
	}

	words := strings.Fields(line)
	if len(words) == 0 {
		return
	}

	lineLen := 0
	for i, word := range words {
		wLen := len(word)
		if i == 0 {
			buf.WriteString(word)
			lineLen = wLen
			continue
		}
		if lineLen+1+wLen > width {
			buf.WriteByte('\n')
			buf.WriteString(word)
			lineLen = wLen
		} else {
			buf.WriteByte(' ')
			buf.WriteString(word)
			lineLen += 1 + wLen
		}
	}
}

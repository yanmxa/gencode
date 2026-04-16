// Tool result display rendering — lipgloss-based formatting for tool output.
// Separated from internal/tool/toolresult to keep the tool layer UI-free.
package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tool/toolresult"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Lipgloss styles for tool result rendering, referencing theme directly.
var (
	headerStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.CurrentTheme.Border).
			Padding(0, 1)

	headerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.CurrentTheme.Primary)

	headerSubtitleStyle = lipgloss.NewStyle().
				Foreground(theme.CurrentTheme.Text)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Muted)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Muted).
			Width(5).
			Align(lipgloss.Right)

	matchStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Warning).
			Bold(true)

	filePathStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Primary)

	truncatedStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Muted).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(theme.CurrentTheme.Error)

)

// RenderToolResult renders a complete tool result with header and content.
func RenderToolResult(result toolresult.ToolResult, width int) string {
	if !result.Success {
		return renderErrorHeader(result.Metadata.Title, result.Error, width)
	}

	var sb strings.Builder

	sb.WriteString(renderHeader(result.Metadata, width))
	sb.WriteString("\n")

	switch result.Metadata.Title {
	case "Read":
		if len(result.Lines) > 0 {
			sb.WriteString(renderLines(result.Lines, true))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Glob":
		if len(result.Files) > 0 {
			sb.WriteString(renderFileList(result.Files, 20))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Grep":
		if len(result.Lines) > 0 {
			sb.WriteString(renderGrepResults(result.Lines, 30))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "WebFetch":
		if result.Output != "" {
			lines := strings.Split(result.Output, "\n")
			for _, line := range lines {
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	default:
		if result.Output != "" {
			sb.WriteString(result.Output)
		}
	}

	return sb.String()
}

// --- Header rendering ---

func renderHeader(meta toolresult.ResultMetadata, width int) string {
	title := headerTitleStyle.Render(meta.Title)
	subtitle := fmt.Sprintf("%s %s", meta.Icon, headerSubtitleStyle.Render(meta.Subtitle))

	metaParts := make([]string, 0, 6)
	if meta.Size > 0 {
		metaParts = append(metaParts, toolresult.FormatSize(meta.Size))
	}
	if meta.LineCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d lines", meta.LineCount))
	}
	if meta.ItemCount > 0 {
		switch meta.Title {
		case "Glob":
			metaParts = append(metaParts, fmt.Sprintf("%d files", meta.ItemCount))
		case "Grep":
			metaParts = append(metaParts, fmt.Sprintf("%d matches", meta.ItemCount))
		default:
			metaParts = append(metaParts, fmt.Sprintf("%d items", meta.ItemCount))
		}
	}
	if meta.StatusCode > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d OK", meta.StatusCode))
	}
	if meta.Duration > 0 {
		metaParts = append(metaParts, toolresult.FormatDuration(meta.Duration))
	}
	if meta.Truncated {
		metaParts = append(metaParts, truncatedStyle.Render("(truncated)"))
	}
	metaLine := headerMetaStyle.Render(strings.Join(metaParts, " · "))

	content := fmt.Sprintf("%s\n%s\n%s", title, subtitle, metaLine)
	box := headerStyle.Width(capBoxWidth(width) - 4).Render(content)
	return box
}

func renderErrorHeader(toolName, errorMsg string, width int) string {
	title := headerTitleStyle.Render(toolName)
	errorLine := fmt.Sprintf("%s %s", toolresult.IconError, errorStyle.Render("Error"))
	msgLine := errorStyle.Render(errorMsg)

	content := fmt.Sprintf("%s\n%s\n%s", title, errorLine, msgLine)

	errorBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.CurrentTheme.Error).
		Padding(0, 1)

	box := errorBoxStyle.Width(capBoxWidth(width) - 4).Render(content)
	return box
}

func capBoxWidth(width int) int {
	if width <= 0 {
		return 50
	}
	maxWidth := width * 80 / 100
	if maxWidth < 50 {
		return 50
	}
	return maxWidth
}

// --- Content rendering ---

func renderLines(lines []toolresult.ContentLine, showLineNo bool) string {
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder

	maxLineNo := 0
	for _, line := range lines {
		if line.LineNo > maxLineNo {
			maxLineNo = line.LineNo
		}
	}
	lineNoWidth := len(fmt.Sprintf("%d", maxLineNo))
	if lineNoWidth < 4 {
		lineNoWidth = 4
	}

	for _, line := range lines {
		switch line.Type {
		case toolresult.LineTruncated:
			sb.WriteString(truncatedStyle.Render(line.Text))
			sb.WriteString("\n")
		default:
			if showLineNo && line.LineNo > 0 {
				lineNoStr := fmt.Sprintf("%*d", lineNoWidth, line.LineNo)
				sb.WriteString(lineNumberStyle.Render(lineNoStr))
				sb.WriteString(lineNumberStyle.Render("│"))
			} else if showLineNo {
				sb.WriteString(strings.Repeat(" ", lineNoWidth))
				sb.WriteString(lineNumberStyle.Render("│"))
			}

			var content string
			switch line.Type {
			case toolresult.LineMatch:
				content = matchStyle.Render(line.Text)
			case toolresult.LineHeader:
				content = filePathStyle.Render(line.Text)
			default:
				content = line.Text
			}
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func renderFileList(files []string, maxShow int) string {
	if len(files) == 0 {
		return truncatedStyle.Render("  (no files found)\n")
	}

	var sb strings.Builder
	showCount := len(files)
	truncated := false
	if maxShow > 0 && showCount > maxShow {
		showCount = maxShow
		truncated = true
	}

	for i := 0; i < showCount; i++ {
		sb.WriteString("  ")
		sb.WriteString(filePathStyle.Render(files[i]))
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(files) - maxShow
		sb.WriteString(truncatedStyle.Render(fmt.Sprintf("  ... and %d more files\n", remaining)))
	}

	return sb.String()
}

func renderGrepResults(lines []toolresult.ContentLine, maxShow int) string {
	if len(lines) == 0 {
		return truncatedStyle.Render("  (no matches found)\n")
	}

	var sb strings.Builder
	showCount := len(lines)
	truncated := false
	if maxShow > 0 && showCount > maxShow {
		showCount = maxShow
		truncated = true
	}

	for i := 0; i < showCount; i++ {
		line := lines[i]
		sb.WriteString("  ")
		if line.File != "" {
			sb.WriteString(filePathStyle.Render(line.File))
			sb.WriteString(":")
		}
		if line.LineNo > 0 {
			sb.WriteString(lineNumberStyle.Render(fmt.Sprintf("%d", line.LineNo)))
			sb.WriteString(": ")
		}
		sb.WriteString(line.Text)
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(lines) - maxShow
		sb.WriteString(truncatedStyle.Render(fmt.Sprintf("  ... and %d more matches\n", remaining)))
	}

	return sb.String()
}

package ui

import (
	"fmt"
	"strings"
)

// LineType represents the type of content line
type LineType int

const (
	LineNormal    LineType = iota // Normal line
	LineMatch                     // Matched line (highlight)
	LineHeader                    // File header
	LineTruncated                 // Truncated indicator
)

// ContentLine represents a formatted content line
type ContentLine struct {
	LineNo int      // Line number (0 means no line number)
	Text   string   // Line content
	Type   LineType // Line type
	File   string   // File path (for grep results)
}

// RenderLines renders content lines with optional line numbers
//
//	1│package main
//	2│
//	3│import "fmt"
func RenderLines(lines []ContentLine, showLineNo bool) string {
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder

	// Calculate max line number width
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
		case LineTruncated:
			sb.WriteString(TruncatedStyle.Render(line.Text))
			sb.WriteString("\n")
		default:
			if showLineNo && line.LineNo > 0 {
				lineNoStr := fmt.Sprintf("%*d", lineNoWidth, line.LineNo)
				sb.WriteString(LineNumberStyle.Render(lineNoStr))
				sb.WriteString(LineNumberStyle.Render("│"))
			} else if showLineNo {
				sb.WriteString(strings.Repeat(" ", lineNoWidth))
				sb.WriteString(LineNumberStyle.Render("│"))
			}

			// Apply styling based on line type
			var content string
			switch line.Type {
			case LineMatch:
				content = MatchStyle.Render(line.Text)
			case LineHeader:
				content = FilePathStyle.Render(line.Text)
			default:
				content = line.Text
			}
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// RenderFileList renders a list of file paths
//
//	src/main.go
//	src/utils/helper.go
func RenderFileList(files []string, maxShow int) string {
	if len(files) == 0 {
		return TruncatedStyle.Render("  (no files found)\n")
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
		sb.WriteString(FilePathStyle.Render(files[i]))
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(files) - maxShow
		sb.WriteString(TruncatedStyle.Render(fmt.Sprintf("  ... and %d more files\n", remaining)))
	}

	return sb.String()
}

// RenderGrepResults renders grep search results
//
//	main.go:42: // TODO: fix this
//	utils.go:15: // TODO: refactor
func RenderGrepResults(lines []ContentLine, maxShow int) string {
	if len(lines) == 0 {
		return TruncatedStyle.Render("  (no matches found)\n")
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
			sb.WriteString(FilePathStyle.Render(line.File))
			sb.WriteString(":")
		}
		if line.LineNo > 0 {
			sb.WriteString(LineNumberStyle.Render(fmt.Sprintf("%d", line.LineNo)))
			sb.WriteString(": ")
		}
		sb.WriteString(line.Text)
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(lines) - maxShow
		sb.WriteString(TruncatedStyle.Render(fmt.Sprintf("  ... and %d more matches\n", remaining)))
	}

	return sb.String()
}

// TruncateText truncates text to maxLen with ellipsis
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return "..."
	}
	return text[:maxLen-3] + "..."
}

// MaxLineLength is the maximum length of a content line
const MaxLineLength = 500

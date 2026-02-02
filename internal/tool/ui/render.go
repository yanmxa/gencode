package ui

import (
	"strconv"
	"strings"
	"time"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success   bool             // Whether the tool succeeded
	Output    string           // Main output content
	Error     string           // Error message if failed
	Metadata  ResultMetadata   // Result metadata
	Lines     []ContentLine    // Formatted content lines (optional)
	Files     []string         // File list (for Glob)
	SkillInfo *SkillResultInfo // Skill-specific info (for Skill tool)
}

// RenderToolResult renders a complete tool result with header and content
func RenderToolResult(result ToolResult, width int) string {
	if !result.Success {
		return RenderErrorHeader(result.Metadata.Title, result.Error, width)
	}

	var sb strings.Builder

	// Render header
	sb.WriteString(RenderHeader(result.Metadata, width))
	sb.WriteString("\n")

	// Render content based on tool type
	switch result.Metadata.Title {
	case "Read":
		if len(result.Lines) > 0 {
			sb.WriteString(RenderLines(result.Lines, true))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Glob":
		if len(result.Files) > 0 {
			sb.WriteString(RenderFileList(result.Files, 20))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Grep":
		if len(result.Lines) > 0 {
			sb.WriteString(RenderGrepResults(result.Lines, 30))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "WebFetch":
		if result.Output != "" {
			// Indent web content
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

// RenderCompactResult renders a compact single-line result
func RenderCompactResult(result ToolResult) string {
	if !result.Success {
		return IconError + " " + ErrorStyle.Render(result.Error)
	}
	return RenderCompactHeader(result.Metadata)
}

// NewSuccessResult creates a success result with metadata
func NewSuccessResult(title, icon, subtitle string, size int64, lineCount, itemCount int, duration time.Duration) ToolResult {
	return ToolResult{
		Success: true,
		Metadata: ResultMetadata{
			Title:     title,
			Icon:      icon,
			Subtitle:  subtitle,
			Size:      size,
			LineCount: lineCount,
			ItemCount: itemCount,
			Duration:  duration,
		},
	}
}

// NewErrorResult creates an error result
func NewErrorResult(title, errorMsg string) ToolResult {
	return ToolResult{
		Success: false,
		Error:   errorMsg,
		Metadata: ResultMetadata{
			Title: title,
		},
	}
}

// FormatForLLM returns a plain text representation of the result for LLM consumption
func (r ToolResult) FormatForLLM() string {
	if !r.Success {
		return "Error: " + r.Error
	}

	var sb strings.Builder

	// Handle different result types
	switch r.Metadata.Title {
	case "Read":
		if len(r.Lines) > 0 {
			for _, line := range r.Lines {
				sb.WriteString(line.Text)
				sb.WriteString("\n")
			}
		} else if r.Output != "" {
			sb.WriteString(r.Output)
		}
	case "Glob":
		if len(r.Files) > 0 {
			for _, f := range r.Files {
				sb.WriteString(f)
				sb.WriteString("\n")
			}
		} else if r.Output != "" {
			sb.WriteString(r.Output)
		}
	case "Grep":
		if len(r.Lines) > 0 {
			for _, line := range r.Lines {
				if line.File != "" {
					sb.WriteString(line.File)
					sb.WriteString(":")
				}
				if line.LineNo > 0 {
					sb.WriteString(strconv.Itoa(line.LineNo))
					sb.WriteString(":")
				}
				sb.WriteString(line.Text)
				sb.WriteString("\n")
			}
		} else if r.Output != "" {
			sb.WriteString(r.Output)
		}
	default:
		if r.Output != "" {
			sb.WriteString(r.Output)
		}
	}

	return sb.String()
}

package toolresult

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success      bool             // Whether the tool succeeded
	Output       string           // Main output content
	Error        string           // Error message if failed
	Metadata     ResultMetadata   // Result metadata
	Lines        []ContentLine    // Formatted content lines (optional)
	Files        []string         // File list (for Glob)
	SkillInfo    *SkillResultInfo // Skill-specific info (for Skill tool)
	HookResponse any              // Structured response for PostToolUse hooks (CC-compatible)
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

	switch r.Metadata.Title {
	case "Read":
		if len(r.Lines) > 0 {
			for _, line := range r.Lines {
				fmt.Fprintf(&sb, "%6d\t%s\n", line.LineNo, line.Text)
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

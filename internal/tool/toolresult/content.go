package toolresult

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

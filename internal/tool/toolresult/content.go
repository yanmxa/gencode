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

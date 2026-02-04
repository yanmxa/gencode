package session

import (
	"strings"
	"unicode/utf8"
)

const (
	// MaxTitleLength is the maximum length for a session title
	MaxTitleLength = 60
)

// GenerateTitle generates a title from the first user message
func GenerateTitle(messages []StoredMessage) string {
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content != "" && msg.ToolResult == nil {
			return truncateTitle(msg.Content)
		}
	}
	return "Untitled Session"
}

// truncateTitle truncates a string to MaxTitleLength, breaking at word boundaries
func truncateTitle(s string) string {
	// Normalize whitespace by splitting on any whitespace and rejoining
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) <= MaxTitleLength {
		return s
	}

	// Truncate to MaxTitleLength runes
	runes := []rune(s)
	truncated := string(runes[:MaxTitleLength])

	// Try to break at the last word boundary
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > MaxTitleLength/2 {
		truncated = truncated[:lastSpace]
	}

	return strings.TrimSpace(truncated) + "..."
}

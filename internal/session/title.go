package session

import (
	"strings"
	"unicode/utf8"
)

const (
	// MaxTitleLength is the maximum length for a session title
	MaxTitleLength = 60
)

// MinSubstantiveLength is the minimum character count for a message to be
// considered "substantive" (i.e. not just a greeting like "hi" or "hello").
const MinSubstantiveLength = 6

// GenerateTitle generates a title from the first substantive user text in the
// entries. Messages with 5 or fewer characters (e.g. "hi", "hello") are
// skipped in favour of the next meaningful message. If no substantive message
// exists, the first user text is used as a fallback.
func GenerateTitle(entries []Entry) string {
	var fallback string
	for _, entry := range entries {
		text, ok := extractUserText(entry)
		if !ok {
			continue
		}
		if fallback == "" {
			fallback = text
		}
		if utf8.RuneCountInString(text) >= MinSubstantiveLength {
			return truncateTitle(text)
		}
	}
	if fallback != "" {
		return truncateTitle(fallback)
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

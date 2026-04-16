package session

import (
	"strings"
	"unicode/utf8"
)

const (
	maxTitleLength       = 60
	MinSubstantiveLength = 6
)

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

func truncateTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= maxTitleLength {
		return s
	}
	runes := []rune(s)
	truncated := string(runes[:maxTitleLength])
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxTitleLength/2 {
		truncated = truncated[:lastSpace]
	}
	return strings.TrimSpace(truncated) + "..."
}

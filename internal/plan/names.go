// Package plan provides plan mode functionality for GenCode.
// Plan mode allows AI to explore the codebase and create implementation plans
// before making changes.
package plan

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// GeneratePlanName generates a plan name based on task description.
// Format: YYYYMMDD-task-keywords (e.g., "20260129-add-dark-mode")
// If task is empty, uses timestamp only.
func GeneratePlanName(task string) string {
	timestamp := time.Now().Format("20060102")

	if task == "" {
		return timestamp + "-plan"
	}

	// Extract keywords from task
	keywords := extractKeywords(task)
	if len(keywords) == 0 {
		return timestamp + "-plan"
	}

	// Limit to 4 keywords for reasonable length
	if len(keywords) > 4 {
		keywords = keywords[:4]
	}

	return timestamp + "-" + strings.Join(keywords, "-")
}

// GeneratePlanNameFromContent generates a plan name from plan content.
// Useful when creating name from the actual plan summary.
func GeneratePlanNameFromContent(content string) string {
	// Try to extract summary or first heading
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for markdown heading
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") {
			title := strings.TrimPrefix(line, "## ")
			title = strings.TrimPrefix(title, "# ")
			return GeneratePlanName(title)
		}
		// Use first non-empty line if no heading
		if line != "" && !strings.HasPrefix(line, "---") {
			return GeneratePlanName(line)
		}
	}
	return GeneratePlanName("")
}

// extractKeywords extracts meaningful keywords from a task description
func extractKeywords(text string) []string {
	text = strings.ToLower(text)

	// Extract alphanumeric words
	wordPattern := regexp.MustCompile(`[a-z0-9]+`)
	words := wordPattern.FindAllString(text, -1)

	keywords := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		if len(word) < 2 || isStopWord(word) || seen[word] {
			continue
		}
		seen[word] = true
		keywords = append(keywords, word)
	}

	return keywords
}

// isStopWord checks if a word is a common English stop word
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"to": true, "for": true, "of": true, "in": true, "on": true,
		"with": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "can": true, "this": true, "that": true, "these": true,
		"those": true, "i": true, "you": true, "we": true, "they": true,
		"it": true, "its": true, "my": true, "your": true, "our": true,
		"their": true, "what": true, "which": true, "who": true, "whom": true,
		"how": true, "when": true, "where": true, "why": true, "all": true,
		"each": true, "every": true, "both": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true,
		"not": true, "only": true, "same": true, "so": true, "than": true,
		"too": true, "very": true, "just": true, "also": true, "now": true,
		"please": true, "help": true, "me": true, "want": true, "need": true,
		"like": true, "make": true, "get": true, "let": true, "using": true,
	}
	return stopWords[word]
}

// SanitizeName ensures a name is valid for filesystem use
func SanitizeName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Keep only alphanumeric and hyphens
	var result strings.Builder
	lastHyphen := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(unicode.ToLower(r))
			lastHyphen = false
		} else if r == '-' && !lastHyphen && result.Len() > 0 {
			result.WriteRune('-')
			lastHyphen = true
		}
	}

	// Trim trailing hyphen
	s := result.String()
	return strings.TrimSuffix(s, "-")
}

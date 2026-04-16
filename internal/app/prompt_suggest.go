package app

import (
	"context"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// promptSuggestionMsg carries the result of a background suggestion generation.
type promptSuggestionMsg struct {
	text string
	err  error
}

// promptSuggestionState holds ghost text suggestion state.
type promptSuggestionState struct {
	text   string
	cancel context.CancelFunc
}

func (s *promptSuggestionState) Clear() {
	s.text = ""
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

const suggestionSystemPrompt = `You predict what the user will type next in a coding assistant CLI.
Reply with ONLY the predicted text (2-12 words). No quotes, no explanation.
If unsure, reply with nothing.`

const suggestionUserPrompt = `[PREDICTION MODE] Based on this conversation, predict what the user will type next.
Stay silent if the next step isn't obvious. Match the user's language and style.`

const maxSuggestionMessages = 20

// startPromptSuggestion launches a background API call to generate a prompt suggestion.
func (m *model) startPromptSuggestion() tea.Cmd {
	req, ok := m.buildPromptSuggestionRequest()
	if !ok {
		return nil
	}

	// Cancel any prior in-flight suggestion
	m.promptSuggestion.Clear()

	ctx, cancel := context.WithCancel(context.Background())
	m.promptSuggestion.cancel = cancel
	req.Ctx = ctx

	return m.asyncOps.SuggestPromptCmd(req)
}

// handlePromptSuggestion processes the suggestion result.
func (m *model) handlePromptSuggestion(msg promptSuggestionMsg) {
	if msg.err != nil {
		return
	}
	// Discard if user already started typing
	if m.input.Textarea.Value() != "" {
		return
	}
	// Discard if streaming is active
	if m.conv.Stream.Active {
		return
	}
	if text := filterSuggestion(msg.text); text != "" {
		m.promptSuggestion.text = text
	}
}

var (
	aiVoicePrefixes = []string{
		"i'll", "i will", "let me", "here's", "here is", "here are",
		"i can", "i would", "i think", "i notice", "i'm",
		"that's", "this is", "this will",
		"you can", "you should", "you could",
		"sure,", "of course", "certainly",
	}
	evaluativePhrases = []string{
		"thanks", "thank you", "looks good", "sounds good",
		"that works", "that worked", "that's all",
		"nice", "great", "perfect", "makes sense",
		"awesome", "excellent", "good job",
	}
	prefixedLabelRe     = regexp.MustCompile(`^\w+:\s`)
	multipleSentencesRe = regexp.MustCompile(`[.!?]\s+[A-Z]`)
)

// filterSuggestion validates and cleans a suggestion. Returns "" if invalid.
func filterSuggestion(text string) string {
	text = strings.TrimSpace(text)
	// Remove surrounding quotes if present
	if len(text) >= 2 && (text[0] == '"' && text[len(text)-1] == '"' ||
		text[0] == '\'' && text[len(text)-1] == '\'') {
		text = text[1 : len(text)-1]
		text = strings.TrimSpace(text)
	}

	if text == "" {
		return ""
	}
	if len(text) > 100 {
		return ""
	}
	words := strings.Fields(text)
	if len(words) > 12 {
		return ""
	}
	// Reject single words unless they are common short commands
	if len(words) < 2 && !isAllowedSingleWord(text) {
		return ""
	}
	// Reject markdown or multi-line
	if strings.ContainsAny(text, "*\n") {
		return ""
	}
	// Reject multiple sentences
	if multipleSentencesRe.MatchString(text) {
		return ""
	}
	// Reject prefixed labels like "Action: ..."
	if prefixedLabelRe.MatchString(text) {
		return ""
	}

	lower := strings.ToLower(text)

	// Reject AI voice
	for _, prefix := range aiVoicePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	// Reject evaluative
	for _, phrase := range evaluativePhrases {
		if strings.Contains(lower, phrase) {
			return ""
		}
	}

	return text
}

var allowedSingleWords = map[string]bool{
	"yes": true, "yeah": true, "yep": true, "yup": true,
	"no": true, "sure": true, "ok": true, "okay": true,
	"push": true, "commit": true, "deploy": true,
	"stop": true, "continue": true, "check": true,
	"exit": true, "quit": true,
}

func isAllowedSingleWord(word string) bool {
	if strings.HasPrefix(word, "/") {
		return true
	}
	return allowedSingleWords[strings.ToLower(word)]
}

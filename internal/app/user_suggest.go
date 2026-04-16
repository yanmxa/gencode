package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit/suggest"
)

type promptSuggestionMsg struct {
	text string
	err  error
}

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

func (m *model) startPromptSuggestion() tea.Cmd {
	req, ok := m.buildPromptSuggestionRequest()
	if !ok {
		return nil
	}

	m.promptSuggestion.Clear()

	ctx, cancel := context.WithCancel(context.Background())
	m.promptSuggestion.cancel = cancel
	req.Ctx = ctx

	return suggestPromptCmd(req)
}

func (m *model) handlePromptSuggestion(msg promptSuggestionMsg) {
	if msg.err != nil {
		return
	}
	if m.userInput.Textarea.Value() != "" {
		return
	}
	if m.conv.Stream.Active {
		return
	}
	if text := suggest.FilterSuggestion(msg.text); text != "" {
		m.promptSuggestion.text = text
	}
}

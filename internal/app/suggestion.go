package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/setting"
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

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *llm.Client
	Messages     []core.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type compactRequest struct {
	Ctx            context.Context
	Client         *llm.Client
	Messages       []core.Message
	SessionSummary string
	Focus          string
	HookEngine     *hook.Engine
	Trigger        string
}

func suggestPromptCmd(req promptSuggestionRequest) tea.Cmd {
	if req.Client == nil {
		return nil
	}
	return func() tea.Msg {
		resp, err := req.Client.Complete(req.Ctx, req.SystemPrompt, req.Messages, req.MaxTokens)
		if err != nil {
			return promptSuggestionMsg{err: err}
		}
		return promptSuggestionMsg{text: resp.Content}
	}
}

func compactCmd(req compactRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := req.Ctx
		focus := req.Focus
		if req.HookEngine != nil {
			outcome := req.HookEngine.Execute(ctx, hook.PreCompact, hook.HookInput{
				Trigger:            req.Trigger,
				CustomInstructions: req.Focus,
			})
			if outcome.AdditionalContext != "" {
				if focus != "" {
					focus += "\n" + outcome.AdditionalContext
				} else {
					focus = outcome.AdditionalContext
				}
			}
		}
		summary, count, err := conv.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return conv.CompactResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

func (m *model) buildPromptSuggestionRequest() (promptSuggestionRequest, bool) {
	if m.runtime.LLMProvider == nil {
		return promptSuggestionRequest{}, false
	}

	assistantCount := 0
	for _, msg := range m.conv.Messages {
		if msg.Role == core.RoleAssistant {
			assistantCount++
		}
	}
	if assistantCount < 2 {
		return promptSuggestionRequest{}, false
	}

	startIdx := 0
	if len(m.conv.Messages) > maxSuggestionMessages {
		startIdx = len(m.conv.Messages) - maxSuggestionMessages
	}
	msgs := m.conv.ConvertToProviderFrom(startIdx)
	msgs = append(msgs, core.Message{
		Role:    core.RoleUser,
		Content: suggestionUserPrompt,
	})

	return promptSuggestionRequest{
		Client:       m.buildLoopClient(),
		Messages:     msgs,
		SystemPrompt: suggestionSystemPrompt,
		UserPrompt:   suggestionUserPrompt,
		MaxTokens:    60,
	}, true
}

func (m *model) buildCompactRequest(focus, trigger string) compactRequest {
	return compactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLoopClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.runtime.SessionSummary,
		Focus:          focus,
		HookEngine:     m.runtime.HookEngine,
		Trigger:        trigger,
	}
}

func (m *model) getEffectiveInputLimit() int {
	return kit.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return kit.GetMaxTokens(m.runtime.ProviderStore, m.runtime.CurrentModel, setting.DefaultMaxTokens)
}

func (m *model) getContextUsagePercent() float64 {
	return kit.GetContextUsagePercent(m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

func (m *model) shouldAutoCompact() bool {
	return kit.ShouldAutoCompact(m.runtime.LLMProvider, len(m.conv.Messages), m.runtime.InputTokens, m.runtime.ProviderStore, m.runtime.CurrentModel)
}

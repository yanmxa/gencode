// Request types, tea.Cmd builders, and model methods for prompt suggestion,
// token-limit fetching, and conversation compaction.
package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appcompact "github.com/yanmxa/gencode/internal/app/output/compact"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/provider"
)

// --- Request types ---

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *provider.Client
	Messages     []core.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type tokenLimitFetchRequest struct {
	Ctx          context.Context
	LLM          provider.Provider
	Store        *provider.Store
	CurrentModel *provider.CurrentModelInfo
	ModelID      string
	Cwd          string
}

type compactRequest struct {
	Ctx            context.Context
	Client         *provider.Client
	Messages       []core.Message
	SessionSummary string
	Focus          string
	HookEngine     *hooks.Engine
	Trigger        string
}

// --- tea.Cmd builders ---

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

func fetchTokenLimitsCmd(req tokenLimitFetchRequest) tea.Cmd {
	deps := appcompact.AutoFetchTokenLimitsDeps{
		LLM:          req.LLM,
		Store:        req.Store,
		CurrentModel: req.CurrentModel,
		ModelID:      req.ModelID,
		Cwd:          req.Cwd,
	}
	ctx := req.Ctx
	return func() tea.Msg {
		result, err := appcompact.AutoFetchTokenLimits(ctx, deps)
		return appcompact.TokenLimitResultMsg{Result: result, Error: err}
	}
}

func compactCmd(req compactRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := req.Ctx
		focus := req.Focus
		if req.HookEngine != nil {
			outcome := req.HookEngine.Execute(ctx, hooks.PreCompact, hooks.HookInput{
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
		summary, count, err := appcompact.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return appcompact.ResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

// --- Model methods that build requests from state ---

func (m *model) buildPromptSuggestionRequest() (promptSuggestionRequest, bool) {
	if m.llmProvider == nil {
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

func (m *model) buildTokenLimitFetchRequest() tokenLimitFetchRequest {
	return tokenLimitFetchRequest{
		Ctx:          context.Background(),
		LLM:          m.llmProvider,
		Store:        m.providerStore,
		CurrentModel: m.currentModel,
		ModelID:      m.getModelID(),
		Cwd:          m.cwd,
	}
}

func (m *model) buildCompactRequest(focus, trigger string) compactRequest {
	return compactRequest{
		Ctx:            context.Background(),
		Client:         m.buildLoopClient(),
		Messages:       m.conv.ConvertToProvider(),
		SessionSummary: m.sessionSummary,
		Focus:          focus,
		HookEngine:     m.hookEngine,
		Trigger:        trigger,
	}
}

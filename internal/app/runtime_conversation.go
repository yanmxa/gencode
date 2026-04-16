package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appcompact "github.com/yanmxa/gencode/internal/app/compact"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
)

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *llm.Client
	Messages     []core.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type tokenLimitFetchRequest struct {
	Ctx          context.Context
	LLM          llm.Provider
	Store        *llm.Store
	CurrentModel *llm.CurrentModelInfo
	ModelID      string
	Cwd          string
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
		summary, count, err := appcompact.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return appcompact.ResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

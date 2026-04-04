package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appcompact "github.com/yanmxa/gencode/internal/app/compact"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

type conversationRuntime struct{}

type promptSuggestionRequest struct {
	Ctx          context.Context
	Client       *client.Client
	Messages     []message.Message
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type tokenLimitFetchRequest struct {
	LLM          provider.LLMProvider
	Store        *provider.Store
	CurrentModel *provider.CurrentModelInfo
	ModelID      string
	Cwd          string
}

type compactRequest struct {
	Client         *client.Client
	Messages       []message.Message
	SessionSummary string
	Focus          string
	HookEngine     *hooks.Engine
	Trigger        string
}

func newConversationRuntime() conversationRuntime {
	return conversationRuntime{}
}

func (conversationRuntime) SuggestPromptCmd(req promptSuggestionRequest) tea.Cmd {
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

func (conversationRuntime) FetchTokenLimitsCmd(req tokenLimitFetchRequest) tea.Cmd {
	deps := appcompact.AutoFetchTokenLimitsDeps{
		LLM:          req.LLM,
		Store:        req.Store,
		CurrentModel: req.CurrentModel,
		ModelID:      req.ModelID,
		Cwd:          req.Cwd,
	}
	return func() tea.Msg {
		ctx := context.Background()
		result, err := appcompact.AutoFetchTokenLimits(ctx, deps)
		return appcompact.TokenLimitResultMsg{Result: result, Error: err}
	}
}

func (conversationRuntime) CompactCmd(req compactRequest) tea.Cmd {
	if req.HookEngine != nil {
		req.HookEngine.ExecuteAsync(hooks.PreCompact, hooks.HookInput{
			Trigger: req.Trigger,
		})
	}

	return func() tea.Msg {
		ctx := context.Background()
		summary, count, err := appcompact.CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, req.Focus)
		return appcompact.CompactResultMsg{Summary: summary, OriginalCount: count, Error: err}
	}
}

package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appcompact "github.com/yanmxa/gencode/internal/app/compact"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

type conversationRuntime interface {
	SuggestPromptCmd(promptSuggestionRequest) tea.Cmd
	FetchTokenLimitsCmd(tokenLimitFetchRequest) tea.Cmd
	CompactCmd(compactRequest) tea.Cmd
	StartStream(streamRequest) streamStartResult
}

type defaultConversationRuntime struct{}

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

type streamRequest struct {
	Loop     *core.Loop
	Messages []message.Message
}

type streamStartResult struct {
	Cancel context.CancelFunc
	Ch     <-chan message.StreamChunk
}

func newConversationRuntime() conversationRuntime {
	return defaultConversationRuntime{}
}

func (defaultConversationRuntime) SuggestPromptCmd(req promptSuggestionRequest) tea.Cmd {
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

func (defaultConversationRuntime) FetchTokenLimitsCmd(req tokenLimitFetchRequest) tea.Cmd {
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

func (defaultConversationRuntime) CompactCmd(req compactRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
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
		return appcompact.CompactResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

func (defaultConversationRuntime) StartStream(req streamRequest) streamStartResult {
	ctx, cancel := context.WithCancel(context.Background())
	req.Loop.SetMessages(req.Messages)
	return streamStartResult{
		Cancel: cancel,
		Ch:     req.Loop.Stream(ctx),
	}
}

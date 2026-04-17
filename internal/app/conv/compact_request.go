package conv

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
)

type CompactRequest struct {
	Ctx            context.Context
	Client         *llm.Client
	Messages       []core.Message
	SessionSummary string
	Focus          string
	HookEngine     *hook.Engine
	Trigger        string
}

func CompactCmd(req CompactRequest) tea.Cmd {
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
		summary, count, err := CompactConversation(ctx, req.Client, req.Messages, req.SessionSummary, focus)
		return CompactResultMsg{Summary: summary, OriginalCount: count, Trigger: req.Trigger, Error: err}
	}
}

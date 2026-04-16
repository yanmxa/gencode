package system

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
)

// Runtime defines the app callbacks needed to process system-originated input.
type Runtime interface {
	IsInputIdle() bool
	InjectCronPrompt(prompt string) tea.Cmd
	InjectAsyncHookContinuation(item AsyncHookRewake) tea.Cmd
	AppendMessage(msg core.ChatMessage)
}

// Update routes Source 3 (system -> agent) messages for the app runtime.
func Update(rt Runtime, state *State, hookEngine *hooks.Engine, msg tea.Msg) (tea.Cmd, bool) {
	switch msg.(type) {
	case CronTickMsg:
		return handleCronTick(rt, state), true
	case AsyncHookTickMsg:
		return handleAsyncHookTick(rt, state, hookEngine), true
	default:
		return nil, false
	}
}

func handleCronTick(rt Runtime, state *State) tea.Cmd {
	result := state.HandleCronTick(rt.IsInputIdle())

	cmds := []tea.Cmd{StartCronTicker()}
	if result.InjectPrompt != "" {
		cmds = append(cmds, rt.InjectCronPrompt(result.InjectPrompt))
	}
	for _, notice := range result.Notices {
		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: notice})
	}
	return tea.Batch(cmds...)
}

func handleAsyncHookTick(rt Runtime, state *State, hookEngine *hooks.Engine) tea.Cmd {
	cmds := []tea.Cmd{StartAsyncHookTicker()}

	item := state.HandleAsyncHookTick(hookEngine, rt.IsInputIdle())
	if item == nil {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, rt.InjectAsyncHookContinuation(*item))
	return tea.Batch(cmds...)
}

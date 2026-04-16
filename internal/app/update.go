// Bubble Tea Update: top-level message dispatch.
package app

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/ui/agentui"
	"github.com/yanmxa/gencode/internal/app/ui/selector"
	"github.com/yanmxa/gencode/internal/app/ui/skillui"
	"github.com/yanmxa/gencode/internal/app/ui/toolui"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/ext/skill"
)

// initialPromptMsg is sent from Init() to inject an initial CLI prompt.
type initialPromptMsg string

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Input & UI chrome ────────────────────────────────────
	switch msg := msg.(type) {
	case initialPromptMsg:
		m.userInput.Textarea.SetValue(string(msg))
		return m, m.handleSubmit()
	case tea.KeyMsg:
		if c, ok := m.handleKeypress(msg); ok {
			return m, c
		}
	case tea.WindowSizeMsg:
		return m, m.handleWindowResize(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		return m, cmd
	case skillui.InvokeMsg:
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(m, sk, "")
			return m, m.handleSkillInvocation()
		}
		return m, nil
	case ctrlOSingleTickMsg:
		return m, m.handleCtrlOSingleTick()
	case promptSuggestionMsg:
		m.handlePromptSuggestion(msg)
		return m, nil
	case selector.DismissedMsg, toolui.ToggleMsg, skillui.CycleMsg, agentui.ToggleMsg:
		return m, nil
	}

	// ── Feature routing ──────────────────────────────────────
	if cmd, handled := m.routeFeatureUpdate(msg); handled {
		return m, cmd
	}
	// ── Fallthrough: forward to textarea & spinner ────────────
	return m, m.updateTextarea(msg)
}

// updateTextarea forwards unhandled messages to the textarea and spinner.
func (m *model) updateTextarea(msg tea.Msg) tea.Cmd {
	cmd, changed := appuser.HandleTextareaUpdate(&m.userInput, msg)
	cmds := []tea.Cmd{cmd}
	if changed {
		m.promptSuggestion.Clear()
	}

	if m.conv.Stream.Active || m.provider.FetchingLimits || m.conv.Compact.Active {
		var spinnerCmd tea.Cmd
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		spinnerCmd = cmd
		cmds = append(cmds, spinnerCmd)
	}

	return tea.Batch(cmds...)
}

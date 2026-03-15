// Bubble Tea Update: top-level message dispatch.
package app

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appskill "github.com/yanmxa/gencode/internal/app/skill"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Input & UI chrome ────────────────────────────────────
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c, ok := m.handleKeypress(msg); ok {
			return m, c
		}
	case tea.WindowSizeMsg:
		return m, m.handleWindowResize(msg)
	case spinner.TickMsg:
		return m, m.handleSpinnerTick(msg)
	case appskill.InvokeMsg:
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(&m, sk, "")
			return m, m.handleSkillInvocation()
		}
		return m, nil
	case ctrlOSingleTickMsg:
		return m, m.handleCtrlOSingleTick()
	case shared.DismissedMsg, apptool.ToggleMsg, appskill.CycleMsg, appagent.ToggleMsg:
		return m, nil
	}

	// ── Feature routing ──────────────────────────────────────
	if c, ok := m.updateStream(msg); ok {
		return m, c
	}
	if c, ok := m.updateTool(msg); ok {
		return m, c
	}
	if c, ok := m.updateApproval(msg); ok {
		return m, c
	}
	if c, ok := m.updateMode(msg); ok {
		return m, c
	}
	if c, ok := m.updateCompact(msg); ok {
		return m, c
	}
	if c, ok := m.updateProvider(msg); ok {
		return m, c
	}
	if c, ok := m.updateMCP(msg); ok {
		return m, c
	}
	if c, ok := m.updatePlugin(msg); ok {
		return m, c
	}
	if c, ok := m.updateSession(msg); ok {
		return m, c
	}
	if c, ok := m.updateMemory(msg); ok {
		return m, c
	}
	// ── Fallthrough: forward to textarea & spinner ────────────
	return m, m.updateTextarea(msg)
}

// updateTextarea forwards unhandled messages to the textarea and spinner.
func (m *model) updateTextarea(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	prevValue := m.input.Textarea.Value()
	m.input.Textarea, cmd = m.input.Textarea.Update(msg)
	cmds = append(cmds, cmd)

	if m.input.Textarea.Value() != prevValue {
		m.input.UpdateHeight()
		m.input.Suggestions.UpdateSuggestions(m.input.Textarea.Value())
	}

	if m.conv.Stream.Active || m.provider.FetchingLimits || m.conv.Compact.Active {
		m.output.Spinner, cmd = m.output.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

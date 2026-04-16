// Bubble Tea Update: top-level message dispatch.
package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/agentui"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/app/selector"
	"github.com/yanmxa/gencode/internal/app/skillui"
	"github.com/yanmxa/gencode/internal/app/toolui"
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
	var cmds []tea.Cmd
	var cmd tea.Cmd

	isPaste := false
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		isPaste = keyMsg.Paste
	}

	prevValue := m.userInput.Textarea.Value()
	m.userInput.Textarea, cmd = m.userInput.Textarea.Update(msg)
	cmds = append(cmds, cmd)

	if isPaste {
		newValue := m.userInput.Textarea.Value()
		pastedText := appuser.ExtractPastedText(prevValue, newValue)
		lines := strings.Split(pastedText, "\n")
		if len(lines) > 1 {
			chunk := appuser.PastedChunk{
				Text:      pastedText,
				LineCount: len(lines),
			}
			m.userInput.PastedChunks = append(m.userInput.PastedChunks, chunk)
			placeholder := appuser.PastePlaceholder(len(m.userInput.PastedChunks), chunk.LineCount)
			m.userInput.Textarea.SetValue(prevValue)
			m.userInput.Textarea.CursorEnd()
			m.userInput.Textarea.InsertString(placeholder)
		} else {
			trimmed := strings.TrimSpace(newValue)
			if trimmed != newValue {
				m.userInput.Textarea.SetValue(trimmed)
				m.userInput.Textarea.CursorEnd()
			}
		}
	}

	if m.userInput.Textarea.Value() != prevValue {
		m.promptSuggestion.Clear()
		m.userInput.UpdateHeight()
		m.userInput.Suggestions.UpdateSuggestions(m.userInput.Textarea.Value())
	}

	if m.conv.Stream.Active || m.provider.FetchingLimits || m.conv.Compact.Active {
		m.agentOutput.Spinner, cmd = m.agentOutput.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}


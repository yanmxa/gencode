// Bubble Tea Update: top-level message dispatch.
package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/agentui"
	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/skillui"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/ui/selector"
)

// initialPromptMsg is sent from Init() to inject an initial CLI prompt.
type initialPromptMsg string

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Input & UI chrome ────────────────────────────────────
	switch msg := msg.(type) {
	case initialPromptMsg:
		m.input.Textarea.SetValue(string(msg))
		return m, m.handleSubmit()
	case tea.KeyMsg:
		if c, ok := m.handleKeypress(msg); ok {
			return m, c
		}
	case tea.WindowSizeMsg:
		return m, m.handleWindowResize(msg)
	case spinner.TickMsg:
		return m, m.handleSpinnerTick(msg)
	case skillui.InvokeMsg:
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(&m, sk, "")
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

	prevValue := m.input.Textarea.Value()
	m.input.Textarea, cmd = m.input.Textarea.Update(msg)
	cmds = append(cmds, cmd)

	if isPaste {
		newValue := m.input.Textarea.Value()
		pastedText := extractPastedText(prevValue, newValue)
		lines := strings.Split(pastedText, "\n")
		if len(lines) > 1 {
			chunk := appinput.PastedChunk{
				Text:      pastedText,
				LineCount: len(lines),
			}
			m.input.PastedChunks = append(m.input.PastedChunks, chunk)
			placeholder := appinput.PastePlaceholder(len(m.input.PastedChunks), chunk.LineCount)
			m.input.Textarea.SetValue(prevValue)
			m.input.Textarea.CursorEnd()
			m.input.Textarea.InsertString(placeholder)
		} else {
			trimmed := strings.TrimSpace(newValue)
			if trimmed != newValue {
				m.input.Textarea.SetValue(trimmed)
				m.input.Textarea.CursorEnd()
			}
		}
	}

	if m.input.Textarea.Value() != prevValue {
		m.promptSuggestion.Clear()
		m.input.UpdateHeight()
		m.input.Suggestions.UpdateSuggestions(m.input.Textarea.Value())
	}

	if m.conv.Stream.Active || m.provider.FetchingLimits || m.conv.Compact.Active {
		m.output.Spinner, cmd = m.output.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// extractPastedText derives the pasted content by comparing the textarea
// value before and after the paste event.
func extractPastedText(prevValue, newValue string) string {
	if strings.HasPrefix(newValue, prevValue) {
		return strings.TrimSpace(newValue[len(prevValue):])
	}
	return strings.TrimSpace(newValue)
}

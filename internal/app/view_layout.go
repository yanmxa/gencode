// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/render"
	"github.com/yanmxa/gencode/internal/app/theme"
)

var ghostTextStyle = lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)

func (m *model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// Render full-screen selectors if any are active.
	if selectorView := m.renderOverlaySelector(); selectorView != "" {
		return selectorView
	}

	separator := render.SeparatorStyle.Render(strings.Repeat("─", m.width))
	trackerView := m.renderTrackerList()
	trackerPrefix := ""
	if trackerView != "" {
		trackerPrefix = "\n" + strings.TrimSuffix(trackerView, "\n") + "\n"
	}

	if modalView := m.renderActiveModal(separator, trackerPrefix); modalView != "" {
		return modalView
	}

	activeContent := m.renderActiveContent()
	inputView := m.renderInputView()
	chatSection := m.renderChatSection(activeContent, trackerView)
	statusLine := m.renderModeStatus()
	suggestions := m.input.Suggestions.Render(m.width)
	tokenWarning := render.RenderTokenWarning(m.provider.InputTokens, m.getEffectiveInputLimit(), m.conv.Compact.WarningSuppressed)
	queuePreview := m.renderQueuePreview()

	var view strings.Builder
	if chatSection != "" {
		view.WriteString(chatSection)
	}
	if tokenWarning != "" {
		view.WriteString("\n")
		view.WriteString(tokenWarning)
	}
	view.WriteString("\n")
	view.WriteString(separator)
	if queuePreview != "" {
		view.WriteString("\n")
		view.WriteString(queuePreview)
	}
	view.WriteString("\n")
	view.WriteString(inputView)
	if suggestions != "" {
		view.WriteString("\n")
		view.WriteString(suggestions)
	}
	view.WriteString("\n")
	view.WriteString(separator)
	view.WriteString("\n")
	if statusLine != "" {
		view.WriteString(statusLine)
	} else {
		view.WriteString(" ")
	}

	return view.String()
}

func (m model) renderInputView() string {
	prompt := render.InputPromptStyle.Render("❯ ")
	if m.promptSuggestion.text != "" && m.input.Textarea.Value() == "" &&
		!m.conv.Stream.Active && !m.input.Suggestions.IsVisible() {
		return prompt + ghostTextStyle.Render(m.promptSuggestion.text)
	}
	return prompt + m.input.RenderTextarea()
}

func (m model) renderChatSection(activeContent, trackerView string) string {
	var parts []string

	if activeContent != "" {
		parts = append(parts, activeContent)
	}

	if trackerView != "" {
		parts = append(parts, strings.TrimSuffix(trackerView, "\n"))
	}

	if m.provider.FetchingLimits {
		spinnerView := render.ThinkingStyle.Render(m.output.Spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	if compactView := render.RenderCompactStatus(
		m.width,
		m.output.Spinner.View(),
		m.conv.Compact.Active,
		m.conv.Compact.Focus,
		m.conv.Compact.Phase,
		m.conv.Compact.LastResult,
		m.conv.Compact.LastError,
	); compactView != "" {
		parts = append(parts, compactView)
	}

	return strings.Join(parts, "\n")
}

// renderTrackerList renders a compact task list above the input area.
// Returns empty string when task display is toggled off via Ctrl+T.
func (m model) renderTrackerList() string {
	if !m.showTasks {
		return ""
	}
	return render.RenderTrackerList(render.TrackerListParams{
		StreamActive: m.conv.Stream.Active,
		Width:        m.width,
		SpinnerView:  m.output.Spinner.View(),
	})
}

func (m model) renderWelcome() string {
	return render.RenderWelcome()
}

func (m model) renderModeStatus() string {
	modelName := m.provider.StatusMessage
	if m.systemInput.HookStatus != "" {
		modelName = m.systemInput.HookStatus
	}
	return render.RenderModeStatus(render.OperationModeParams{
		Mode:          m.mode.Operation,
		InputTokens:   m.provider.InputTokens,
		InputLimit:    m.getEffectiveInputLimit(),
		ModelName:     modelName,
		Width:         m.width,
		ThinkingLevel: m.effectiveThinkingLevel(),
		QueueCount:    m.inputQueue.Len(),
	})
}

func (m model) renderQueuePreview() string {
	items := m.inputQueue.Items()
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSuffix(render.RenderQueuePreview(items, m.queueSelectIdx, m.width), "\n")
}

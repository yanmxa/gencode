// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

var ghostTextStyle = lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)

func (m *model) View() string {
	if !m.env.Ready {
		return "\n  Loading..."
	}

	// Render full-screen selectors if any are active.
	if selectorView := m.renderOverlaySelector(); selectorView != "" {
		return selectorView
	}

	separator := conv.SeparatorStyle.Render(strings.Repeat("─", m.env.Width))
	trackerView := m.renderTrackerList()
	trackerPrefix := ""
	if trackerView != "" {
		trackerPrefix = "\n" + strings.TrimSuffix(trackerView, "\n") + "\n"
	}

	if modalView := m.renderActiveModal(separator, trackerPrefix); modalView != "" {
		return modalView
	}

	activeContent := conv.RenderActiveContent(m.messageRenderParams())
	inputView := m.renderInputView()
	chatSection := m.renderChatSection(activeContent, trackerView)
	statusLine := m.renderModeStatus()
	suggestions := m.userInput.Suggestions.Render(m.env.Width)
	tokenWarning := conv.RenderTokenWarning(m.env.InputTokens, kit.GetEffectiveInputLimit(m.services.LLM.Store(), m.env.CurrentModel), m.conv.Compact.WarningSuppressed)
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

func (m *model) renderOverlaySelector() string {
	for _, s := range m.overlaySelectors() {
		if s.IsActive() {
			return s.Render()
		}
	}
	return ""
}

func (m *model) renderActiveModal(separator, trackerPrefix string) string {
	switch {
	case m.userInput.Approval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.userInput.Approval.Render())
	case m.conv.Modal.Question.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.conv.Modal.Question.Render())
	default:
		return ""
	}
}

func separatorWrapped(trackerPrefix, separator, content string) string {
	return trackerPrefix + separator + "\n" + content
}

func (m model) renderInputView() string {
	prompt := conv.InputPromptStyle.Render("❯ ")
	if m.userInput.PromptSuggestion.Text != "" && m.userInput.Textarea.Value() == "" &&
		!m.conv.Stream.Active && !m.userInput.Suggestions.IsVisible() {
		return prompt + ghostTextStyle.Render(m.userInput.PromptSuggestion.Text)
	}
	return prompt + m.userInput.RenderTextarea()
}

func (m model) renderChatSection(activeContent, trackerView string) string {
	var parts []string

	if activeContent != "" {
		parts = append(parts, activeContent)
	}

	if trackerView != "" {
		parts = append(parts, strings.TrimSuffix(trackerView, "\n"))
	}

	if m.userInput.Provider.FetchingLimits {
		spinnerView := conv.ThinkingStyle.Render(m.conv.Spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	if compactView := conv.RenderCompactStatus(m.env.Width, m.conv.Spinner.View(), m.conv.Compact); compactView != "" {
		parts = append(parts, compactView)
	}

	return strings.Join(parts, "\n")
}

func (m model) renderTrackerList() string {
	if !m.conv.ShowTasks {
		return ""
	}
	tasks := m.services.Tracker.List()
	return conv.RenderTrackerList(conv.TrackerListParams{
		Tasks:        tasks,
		AllDone:      m.services.Tracker.AllDone(),
		StreamActive: m.conv.Stream.Active,
		Width:        m.env.Width,
		SpinnerView:  m.conv.Spinner.View(),
		Blockers:     m.services.Tracker.OpenBlockers,
	})
}

func (m model) renderModeStatus() string {
	modelName := m.env.GetModelID()
	if m.services.Hook != nil {
		if status := m.services.Hook.CurrentStatusMessage(); status != "" {
			modelName = status
		}
	}
	return conv.RenderModeStatus(conv.OperationModeParams{
		Mode:          conv.OperationMode(m.env.OperationMode),
		InputTokens:   m.env.InputTokens,
		OutputTokens:  m.env.OutputTokens,
		InputLimit:    kit.GetEffectiveInputLimit(m.services.LLM.Store(), m.env.CurrentModel),
		ModelName:     modelName,
		Width:         m.env.Width,
		ThinkingLevel: m.env.EffectiveThinkingLevel(),
		QueueCount:    m.userInput.Queue.Len(),
	})
}

func (m model) renderQueuePreview() string {
	items := m.userInput.Queue.Items()
	if len(items) == 0 {
		return ""
	}
	previews := make([]conv.QueuePreviewItem, len(items))
	for i, item := range items {
		previews[i] = conv.QueuePreviewItem{Content: item.Content, HasImages: len(item.Images) > 0}
	}
	return strings.TrimSuffix(conv.RenderQueuePreview(previews, m.userInput.Queue.SelectIdx, m.env.Width), "\n")
}

func (m model) messageRenderParams() conv.MessageRenderParams {
	return conv.MessageRenderParams{
		Messages:                m.conv.Messages,
		CommittedCount:          m.conv.CommittedCount,
		StreamActive:            m.conv.Stream.Active,
		BuildingTool:            m.conv.Stream.BuildingTool,
		Width:                   m.env.Width,
		MDRenderer:              m.conv.MDRenderer,
		SpinnerView:             m.conv.Spinner.View(),
		TaskProgress:            m.conv.TaskProgress,
		TaskOwnerMap:            buildTaskOwnerMap(m.services.Tracker.List()),
		InteractivePromptActive: m.conv.Modal.Question != nil && m.conv.Modal.Question.IsActive(),
	}
}

func buildTaskOwnerMap(tasks []*tracker.Task) map[string]string {
	if len(tasks) == 0 {
		return nil
	}
	ownerMap := make(map[string]string, len(tasks))
	for _, t := range tasks {
		if t.Owner != "" {
			ownerMap[t.ID] = t.Owner
		}
	}
	if len(ownerMap) == 0 {
		return nil
	}
	return ownerMap
}

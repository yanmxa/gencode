// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

var ghostTextStyle = lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)

func (m *model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// Render full-screen selectors if any are active.
	if selectorView := m.renderOverlaySelector(); selectorView != "" {
		return selectorView
	}

	separator := conv.SeparatorStyle.Render(strings.Repeat("─", m.width))
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
	suggestions := m.userInput.Suggestions.Render(m.width)
	tokenWarning := conv.RenderTokenWarning(m.runtime.InputTokens, kit.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel), m.conv.Compact.WarningSuppressed)
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
	case m.conv.Modal.PlanApproval != nil && m.conv.Modal.PlanApproval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.conv.Modal.PlanApproval.RenderMenu())
	case m.userInput.Approval.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.userInput.Approval.Render())
	case m.conv.Modal.Question.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.conv.Modal.Question.Render())
	case m.conv.Modal.PlanEntry.IsActive():
		return separatorWrapped(trackerPrefix, separator, m.conv.Modal.PlanEntry.Render())
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

	if compactView := conv.RenderCompactStatus(m.width, m.conv.Spinner.View(), m.conv.Compact); compactView != "" {
		parts = append(parts, compactView)
	}

	return strings.Join(parts, "\n")
}

func (m model) renderTrackerList() string {
	if !m.conv.ShowTasks {
		return ""
	}
	tasks := tracker.DefaultStore.List()
	return conv.RenderTrackerList(conv.TrackerListParams{
		Tasks:        tasks,
		AllDone:      tracker.DefaultStore.AllDone(),
		StreamActive: m.conv.Stream.Active,
		Width:        m.width,
		SpinnerView:  m.conv.Spinner.View(),
		Blockers:     tracker.DefaultStore.OpenBlockers,
		WorkerSnap: func(taskID, agentID string) (*orchestration.Snapshot, bool) {
			return orchestration.DefaultStore.Snapshot(taskID, agentID, "", 1)
		},
	})
}

func (m model) renderModeStatus() string {
	modelName := m.userInput.Provider.StatusMessage
	if m.runtime.HookEngine != nil {
		if status := m.runtime.HookEngine.CurrentStatusMessage(); status != "" {
			modelName = status
		}
	}
	return conv.RenderModeStatus(conv.OperationModeParams{
		Mode:          conv.OperationMode(m.runtime.OperationMode),
		InputTokens:   m.runtime.InputTokens,
		InputLimit:    kit.GetEffectiveInputLimit(m.runtime.ProviderStore, m.runtime.CurrentModel),
		ModelName:     modelName,
		Width:         m.width,
		ThinkingLevel: m.runtime.EffectiveThinkingLevel(),
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
	return strings.TrimSuffix(conv.RenderQueuePreview(previews, m.userInput.Queue.SelectIdx, m.width), "\n")
}

func (m model) messageRenderParams() conv.MessageRenderParams {
	return conv.MessageRenderParams{
		Messages:                m.conv.Messages,
		CommittedCount:          m.conv.CommittedCount,
		StreamActive:            m.conv.Stream.Active,
		BuildingTool:            m.conv.Stream.BuildingTool,
		Width:                   m.width,
		MDRenderer:              m.conv.MDRenderer,
		SpinnerView:             m.conv.Spinner.View(),
		TaskProgress:            m.conv.TaskProgress,
		TaskOwnerMap:            buildTaskOwnerMap(tracker.DefaultStore.List()),
		InteractivePromptActive: (m.conv.Modal.Question != nil && m.conv.Modal.Question.IsActive()) || (m.conv.Modal.PlanApproval != nil && m.conv.Modal.PlanApproval.IsActive()),
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

func (m model) renderPlanForScrollback(req *tool.PlanRequest) string {
	if req == nil {
		return ""
	}
	return conv.RenderPlanForScrollback(req.Plan, m.conv.MDRenderer)
}

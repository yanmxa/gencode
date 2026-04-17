// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/app/output/render"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/kit"
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
	suggestions := m.userInput.Suggestions.Render(m.width)
	tokenWarning := render.RenderTokenWarning(m.inputTokens, m.getEffectiveInputLimit(), m.conv.Compact.WarningSuppressed)
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
	if m.promptSuggestion.text != "" && m.userInput.Textarea.Value() == "" &&
		!m.conv.Stream.Active && !m.userInput.Suggestions.IsVisible() {
		return prompt + ghostTextStyle.Render(m.promptSuggestion.text)
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
		spinnerView := render.ThinkingStyle.Render(m.agentOutput.Spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	if compactView := render.RenderCompactStatus(
		m.width,
		m.agentOutput.Spinner.View(),
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
		SpinnerView:  m.agentOutput.Spinner.View(),
	})
}

func (m model) renderWelcome() string {
	return render.RenderWelcome()
}

func (m model) renderModeStatus() string {
	modelName := appsystem.RenderHookStatus(m.systemInput.HookStatus, m.userInput.Provider.StatusMessage)
	return render.RenderModeStatus(render.OperationModeParams{
		Mode:          m.operationMode,
		InputTokens:   m.inputTokens,
		InputLimit:    m.getEffectiveInputLimit(),
		ModelName:     modelName,
		Width:         m.width,
		ThinkingLevel: m.effectiveThinkingLevel(),
		QueueCount:    m.userInput.Queue.Len(),
	})
}

func (m model) renderQueuePreview() string {
	items := m.userInput.Queue.Items()
	if len(items) == 0 {
		return ""
	}
	previews := make([]render.QueuePreviewItem, len(items))
	for i, item := range items {
		previews[i] = render.QueuePreviewItem{Content: item.Content, HasImages: len(item.Images) > 0}
	}
	return strings.TrimSuffix(render.RenderQueuePreview(previews, m.userInput.QueueSelectIdx, m.width), "\n")
}

// --- Message rendering (thin delegation to output package) ---

func (m model) messageRenderParams() appoutput.MessageRenderParams {
	return appoutput.MessageRenderParams{
		Messages:                m.conv.Messages,
		CommittedCount:          m.conv.CommittedCount,
		StreamActive:            m.conv.Stream.Active,
		BuildingTool:            m.conv.Stream.BuildingTool,
		Width:                   m.width,
		MDRenderer:              m.agentOutput.MDRenderer,
		SpinnerView:             m.agentOutput.Spinner.View(),
		TaskProgress:            m.agentOutput.TaskProgress,
		InteractivePromptActive: (m.mode.Question != nil && m.mode.Question.IsActive()) || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive()),
	}
}

func (m model) renderPlanForScrollback(req *tool.PlanRequest) string {
	if req == nil {
		return ""
	}
	return appoutput.RenderPlanForScrollback(req.Plan, m.agentOutput.MDRenderer)
}

func (m model) renderSingleMessage(idx int) string {
	return appoutput.RenderSingleMessage(m.messageRenderParams(), idx)
}

func (m model) renderActiveContent() string {
	return appoutput.RenderActiveContent(m.messageRenderParams())
}

func (m model) isToolPhaseActive() bool {
	return false
}

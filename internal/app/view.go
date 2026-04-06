// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
// Also includes message rendering: thin model-method wrappers that delegate to the render package.
package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/render"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

var (
	ghostTextStyle = lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
	ghostHintStyle = lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
)

func (m model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// Render full-screen selectors if any are active
	if selectorView := m.renderOverlaySelector(); selectorView != "" {
		return selectorView
	}

	separator := render.SeparatorStyle.Render(strings.Repeat("─", m.width))
	todoView := m.renderTodoList()
	todoPrefix := ""
	if todoView != "" {
		todoPrefix = "\n" + strings.TrimSuffix(todoView, "\n") + "\n"
	}

	if modalView := m.renderActiveModal(separator, todoPrefix); modalView != "" {
		return modalView
	}

	activeContent := m.renderActiveContent()

	prompt := render.InputPromptStyle.Render("❯ ")
	pendingImagesView := m.input.RenderPendingImages()

	var inputView string
	if m.promptSuggestion.text != "" && m.input.Textarea.Value() == "" &&
		!m.conv.Stream.Active && !m.input.Suggestions.IsVisible() {
		inputView = prompt + ghostTextStyle.Render(m.promptSuggestion.text) + "  " + ghostHintStyle.Render("Tab")
	} else {
		inputView = prompt + m.input.Textarea.View()
	}

	var parts []string

	if activeContent != "" {
		parts = append(parts, activeContent)
	}

	if todoView != "" {
		parts = append(parts, strings.TrimSuffix(todoView, "\n"))
	}

	if pendingImagesView != "" {
		parts = append(parts, strings.TrimSuffix(pendingImagesView, "\n"))
	}

	if m.provider.FetchingLimits {
		spinnerView := render.ThinkingStyle.Render(m.output.Spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	if m.conv.Compact.Active {
		spinnerView := render.ThinkingStyle.Render(m.output.Spinner.View() + " Compacting conversation...")
		parts = append(parts, spinnerView)
	}

	chatSection := strings.Join(parts, "\n")

	statusLine := m.renderModeStatus()
	suggestions := m.input.Suggestions.Render(m.width)

	topSeparator := separator

	var view strings.Builder
	if chatSection != "" {
		view.WriteString(chatSection)
	}
	view.WriteString("\n")
	view.WriteString(topSeparator)
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

// renderTodoList renders a compact task list above the input area.
// Returns empty string when task display is toggled off via Ctrl+T.
func (m model) renderTodoList() string {
	if !m.showTasks {
		return ""
	}
	return render.RenderTodoList(render.TodoListParams{
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
	if m.hookStatus != "" {
		modelName = m.hookStatus
	}
	return render.RenderModeStatus(render.OperationModeParams{
		Mode:          int(m.mode.Operation),
		InputTokens:   m.provider.InputTokens,
		InputLimit:    m.getEffectiveInputLimit(),
		ModelName:     modelName,
		Width:         m.width,
		ThinkingLevel: m.effectiveThinkingLevel(),
	})
}

// buildSkipIndices returns a set of message indices that should be skipped during rendering.
// Tool result messages are skipped when they are rendered inline with their tool calls.
func (m model) buildSkipIndices(startIdx int) map[int]bool {
	skipIndices := make(map[int]bool)
	for i := startIdx; i < len(m.conv.Messages); i++ {
		msg := m.conv.Messages[i]
		if msg.Role != message.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		// Mark subsequent tool result messages that match these tool calls
		for j := i + 1; j < len(m.conv.Messages) && m.conv.Messages[j].ToolResult != nil; j++ {
			for _, tc := range msg.ToolCalls {
				if tc.ID == m.conv.Messages[j].ToolResult.ToolCallID {
					skipIndices[j] = true
					break
				}
			}
		}
	}
	return skipIndices
}

// renderPlanForScrollback renders the plan title + markdown content as a styled
// string for pushing into terminal scrollback via tea.Println.
func (m model) renderPlanForScrollback(req *tool.PlanRequest) string {
	if req == nil {
		return ""
	}
	return render.RenderPlanForScrollback(req.Plan, m.output.MDRenderer)
}

// renderSingleMessage renders one message at the given index for committing to scrollback.
// It handles the skip logic for inline tool results.
// The trailing newline is trimmed because tea.Println adds its own.
func (m model) renderSingleMessage(idx int) string {
	if idx < 0 || idx >= len(m.conv.Messages) {
		return ""
	}

	// Skip tool results that are rendered inline with their tool calls
	if m.conv.Messages[idx].ToolResult != nil && m.isToolResultInlined(idx) {
		return ""
	}

	return strings.TrimRight(m.renderMessageAt(idx, false), "\n")
}

// renderActiveContent renders all uncommitted messages for the managed region.
// This includes: assistant messages waiting for tool results, partial tool results,
// streaming assistant message, and pending tool spinner.
func (m model) renderActiveContent() string {
	if m.conv.CommittedCount >= len(m.conv.Messages) {
		return m.renderPendingToolSpinner()
	}
	return m.renderMessageRange(m.conv.CommittedCount, len(m.conv.Messages), true)
}

// isToolResultInlined checks if the tool result at idx was rendered inline with its tool call.
func (m model) isToolResultInlined(idx int) bool {
	msg := m.conv.Messages[idx]
	if msg.ToolResult == nil {
		return false
	}
	toolCallID := msg.ToolResult.ToolCallID

	// Look backwards for the assistant message that has the matching tool call
	for j := idx - 1; j >= 0; j-- {
		prev := m.conv.Messages[j]
		if prev.Role == message.RoleAssistant && len(prev.ToolCalls) > 0 {
			for _, tc := range prev.ToolCalls {
				if tc.ID == toolCallID {
					return true
				}
			}
			// Found an assistant message with tool calls but no match - stop searching
			break
		}
		// Skip over other tool result messages in the sequence
		if prev.ToolResult != nil {
			continue
		}
		// Non-tool-result, non-assistant message breaks the chain
		break
	}
	return false
}

// renderMessageAt renders a single message at the given index.
func (m model) renderMessageAt(idx int, isStreaming bool) string {
	msg := m.conv.Messages[idx]
	var sb strings.Builder

	if msg.ToolResult == nil {
		sb.WriteString("\n")
	}

	switch msg.Role {
	case message.RoleUser:
		if msg.ToolResult != nil {
			sb.WriteString(m.renderToolResult(msg))
		} else {
			sb.WriteString(m.renderUserMessage(msg))
		}
	case message.RoleNotice:
		sb.WriteString(render.RenderSystemMessage(msg.Content))
	case message.RoleAssistant:
		sb.WriteString(m.renderAssistantMessage(msg, idx, isStreaming))
	}

	return sb.String()
}

// renderMessageRange renders messages from startIdx to endIdx with skip logic and spinner.
func (m model) renderMessageRange(startIdx, endIdx int, includeSpinner bool) string {
	skipIndices := m.buildSkipIndices(startIdx)
	var sb strings.Builder

	lastIdx := endIdx - 1
	isLastStreaming := m.conv.Stream.Active && lastIdx >= 0 && m.conv.Messages[lastIdx].Role == message.RoleAssistant

	for i := startIdx; i < endIdx; i++ {
		if skipIndices[i] {
			continue
		}
		isStreaming := i == lastIdx && isLastStreaming
		sb.WriteString(m.renderMessageAt(i, isStreaming))
	}

	if includeSpinner {
		sb.WriteString(m.renderPendingToolSpinner())
	}

	return sb.String()
}

func (m model) renderUserMessage(msg message.ChatMessage) string {
	return render.RenderUserMessage(msg.Content, msg.Images, m.output.MDRenderer, m.width)
}

func (m model) renderToolResult(msg message.ChatMessage) string {
	return m.renderToolResultInline(msg)
}

func (m model) renderAssistantMessage(msg message.ChatMessage, idx int, isLast bool) string {
	// Render the base assistant message (thinking + content)
	base := render.RenderAssistantMessage(render.AssistantParams{
		Content:       msg.Content,
		Thinking:      msg.Thinking,
		ToolCalls:     msg.ToolCalls,
		StreamActive:  m.conv.Stream.Active,
		IsLast:        isLast,
		SpinnerView:   m.output.Spinner.View(),
		MDRenderer:    m.output.MDRenderer,
		Width:         m.width,
		ExecutingTool: m.getExecutingToolName(),
	})

	if len(msg.ToolCalls) == 0 {
		return base
	}

	// Render tool calls with result map
	var sb strings.Builder
	sb.WriteString(base)

	if msg.Content != "" {
		sb.WriteString("\n")
	}

	// Build result map from subsequent messages
	resultMap := make(map[string]render.ToolResultData)
	for j := idx + 1; j < len(m.conv.Messages); j++ {
		nextMsg := m.conv.Messages[j]
		if nextMsg.ToolResult == nil {
			break
		}
		resultMap[nextMsg.ToolResult.ToolCallID] = render.ToolResultData{
			ToolName: nextMsg.ToolName,
			Content:  nextMsg.ToolResult.Content,
			IsError:  nextMsg.ToolResult.IsError,
			Expanded: nextMsg.Expanded,
		}
	}

	// Build parallel results done map
	parallelDone := make(map[int]bool)
	for k := range m.tool.ParallelResults {
		parallelDone[k] = true
	}

	sb.WriteString(render.RenderToolCalls(render.ToolCallsParams{
		ToolCalls:         msg.ToolCalls,
		ToolCallsExpanded: msg.ToolCallsExpanded,
		ResultMap:         resultMap,
		ParallelMode:      m.tool.Parallel,
		ParallelResults:   parallelDone,
		TaskProgress:      m.output.TaskProgress,
		PendingCalls:      m.tool.PendingCalls,
		SpinnerView:       m.output.Spinner.View(),
		TaskOwnerMap:      m.buildTaskOwnerMap(),
		MDRenderer:        m.output.MDRenderer,
	}))

	return sb.String()
}

// renderToolResultInline renders a tool result inline (without leading newline).
func (m model) renderToolResultInline(msg message.ChatMessage) string {
	return render.RenderToolResultInline(render.ToolResultData{
		ToolName: msg.ToolName,
		Content:  msg.ToolResult.Content,
		IsError:  msg.ToolResult.IsError,
		Expanded: msg.Expanded,
	}, m.output.MDRenderer)
}

func (m model) renderPendingToolSpinner() string {
	interactivePromptActive := m.mode.Question.IsActive() || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive())

	return render.RenderPendingToolSpinner(render.PendingToolSpinnerParams{
		InteractivePromptActive: interactivePromptActive,
		ParallelMode:            m.tool.Parallel,
		HasParallelTaskTools:    m.hasParallelTaskTools(),
		BuildingTool:            m.conv.Stream.BuildingTool,
		PendingCalls:            m.tool.PendingCalls,
		CurrentIdx:              m.tool.CurrentIdx,
		TaskProgress:            m.output.TaskProgress,
		SpinnerView:             m.output.Spinner.View(),
	})
}

// getExecutingToolName returns the name of the tool currently being executed, or "".
func (m model) getExecutingToolName() string {
	if m.conv.Stream.BuildingTool != "" {
		return m.conv.Stream.BuildingTool
	}
	if m.tool.PendingCalls != nil && m.tool.CurrentIdx < len(m.tool.PendingCalls) {
		return m.tool.PendingCalls[m.tool.CurrentIdx].Name
	}
	return ""
}

// hasParallelTaskTools returns true if any pending tool call is a Task tool.
func (m model) hasParallelTaskTools() bool {
	for _, tc := range m.tool.PendingCalls {
		if tc.Name == tool.ToolAgent {
			return true
		}
	}
	return false
}

// buildTaskOwnerMap builds a map of task ID → owner name for TaskGet display.
func (m model) buildTaskOwnerMap() map[string]string {
	tasks := tool.DefaultTodoStore.List()
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

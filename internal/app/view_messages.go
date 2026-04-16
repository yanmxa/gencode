// Message rendering helpers for the interactive transcript area.
package app

import (
	"strings"

	"github.com/yanmxa/gencode/internal/app/render"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

// buildSkipIndices returns a set of message indices that should be skipped during rendering.
// Tool result messages are skipped when they are rendered inline with their tool calls.
func (m model) buildSkipIndices(startIdx int) map[int]bool {
	skipIndices := make(map[int]bool)
	for i := startIdx; i < len(m.conv.Messages); i++ {
		msg := m.conv.Messages[i]
		if msg.Role != core.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		for j := i + 1; j < len(m.conv.Messages); j++ {
			if m.conv.Messages[j].Role == core.RoleNotice {
				continue
			}
			if m.conv.Messages[j].ToolResult == nil {
				break
			}
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
		return m.renderPendingToolSpinner(false)
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

	for j := idx - 1; j >= 0; j-- {
		prev := m.conv.Messages[j]
		if prev.Role == core.RoleNotice {
			continue
		}
		if prev.Role == core.RoleAssistant && len(prev.ToolCalls) > 0 {
			for _, tc := range prev.ToolCalls {
				if tc.ID == toolCallID {
					return true
				}
			}
			break
		}
		if prev.ToolResult != nil {
			continue
		}
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
	case core.RoleUser:
		if msg.ToolResult != nil {
			sb.WriteString(m.renderToolResult(msg))
		} else {
			sb.WriteString(m.renderUserMessage(msg))
		}
	case core.RoleNotice:
		sb.WriteString(render.RenderSystemMessage(msg.Content))
	case core.RoleAssistant:
		sb.WriteString(m.renderAssistantMessage(msg, idx, isStreaming))
	}

	return sb.String()
}

// renderMessageRange renders messages from startIdx to endIdx with skip logic and spinner.
func (m model) renderMessageRange(startIdx, endIdx int, includeSpinner bool) string {
	skipIndices := m.buildSkipIndices(startIdx)
	var sb strings.Builder

	lastIdx := endIdx - 1
	isLastStreaming := m.conv.Stream.Active && lastIdx >= 0 && m.conv.Messages[lastIdx].Role == core.RoleAssistant

	for i := startIdx; i < endIdx; i++ {
		if skipIndices[i] {
			continue
		}
		isStreaming := i == lastIdx && isLastStreaming
		sb.WriteString(m.renderMessageAt(i, isStreaming))
	}

	if includeSpinner {
		sb.WriteString(m.renderPendingToolSpinner(startIdx < endIdx))
	}

	return sb.String()
}

func (m model) renderUserMessage(msg core.ChatMessage) string {
	return render.RenderUserMessage(msg.Content, msg.DisplayContent, msg.Images, m.output.MDRenderer, m.width)
}

func (m model) renderToolResult(msg core.ChatMessage) string {
	return m.renderToolResultInline(msg)
}

func (m model) renderAssistantMessage(msg core.ChatMessage, idx int, isLast bool) string {
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

	var sb strings.Builder
	sb.WriteString(base)

	if msg.Content != "" {
		sb.WriteString("\n")
	}

	resultMap := make(map[string]render.ToolResultData)
	for j := idx + 1; j < len(m.conv.Messages); j++ {
		nextMsg := m.conv.Messages[j]
		if nextMsg.Role == core.RoleNotice {
			continue
		}
		if nextMsg.ToolResult == nil {
			break
		}
		resultMap[nextMsg.ToolResult.ToolCallID] = render.ToolResultData{
			ToolName: nextMsg.ToolName,
			Content:  nextMsg.ToolResult.Content,
			Error:    nextMsg.ToolResult.Content,
			IsError:  nextMsg.ToolResult.IsError,
			Expanded: nextMsg.Expanded,
		}
	}

	sb.WriteString(render.RenderToolCalls(render.ToolCallsParams{
		ToolCalls:         msg.ToolCalls,
		ToolCallsExpanded: msg.ToolCallsExpanded,
		ResultMap:         resultMap,
		TaskProgress:      m.output.TaskProgress,
		SpinnerView:       m.output.Spinner.View(),
		TaskOwnerMap:      m.buildTaskOwnerMap(),
		MDRenderer:        m.output.MDRenderer,
		Width:             m.width,
	}))

	return sb.String()
}

// renderToolResultInline renders a tool result inline (without leading newline).
func (m model) renderToolResultInline(msg core.ChatMessage) string {
	return render.RenderToolResultInline(render.ToolResultData{
		ToolName: msg.ToolName,
		Content:  msg.ToolResult.Content,
		Error:    msg.ToolResult.Content,
		IsError:  msg.ToolResult.IsError,
		Expanded: msg.Expanded,
	}, m.output.MDRenderer)
}

func (m model) renderPendingToolSpinner(suppressAgentLabel bool) string {
	interactivePromptActive := m.mode.Question.IsActive() || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive())

	return render.RenderPendingToolSpinner(render.PendingToolSpinnerParams{
		InteractivePromptActive: interactivePromptActive,
		BuildingTool:            m.conv.Stream.BuildingTool,
		TaskProgress:            m.output.TaskProgress,
		SpinnerView:             m.output.Spinner.View(),
		Width:                   m.width,
		SuppressAgentLabel:      suppressAgentLabel,
	})
}

// isToolPhaseActive returns true while the tool execution phase is active.
// In the agent path, tools execute inside the core.Agent goroutine, so this
// is always false — idle gating uses conv.Stream.Active instead.
func (m model) isToolPhaseActive() bool {
	return false
}

// getExecutingToolName returns the name of the tool currently being executed, or "".
func (m model) getExecutingToolName() string {
	return m.conv.Stream.BuildingTool
}

// buildTaskOwnerMap builds a map of task ID → owner name for TaskGet display.
func (m model) buildTaskOwnerMap() map[string]string {
	tasks := tracker.DefaultStore.List()
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

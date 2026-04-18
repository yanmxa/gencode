package conv

import (
	"strings"

	"github.com/yanmxa/gencode/internal/core"
)

type MessageRenderParams struct {
	Messages                []core.ChatMessage
	CommittedCount          int
	StreamActive            bool
	BuildingTool            string
	Width                   int
	MDRenderer              *MDRenderer
	SpinnerView             string
	TaskProgress            map[int][]string
	TaskOwnerMap            map[string]string
	InteractivePromptActive bool
}

// BuildSkipIndices returns a set of message indices that should be skipped during rendering.
// Tool result messages are skipped when they are rendered inline with their tool calls.
func BuildSkipIndices(messages []core.ChatMessage, startIdx int) map[int]bool {
	skipIndices := make(map[int]bool)
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != core.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		for j := i + 1; j < len(messages); j++ {
			if messages[j].Role == core.RoleNotice {
				continue
			}
			if messages[j].ToolResult == nil {
				break
			}
			for _, tc := range msg.ToolCalls {
				if tc.ID == messages[j].ToolResult.ToolCallID {
					skipIndices[j] = true
					break
				}
			}
		}
	}
	return skipIndices
}

func IsToolResultInlined(messages []core.ChatMessage, idx int) bool {
	msg := messages[idx]
	if msg.ToolResult == nil {
		return false
	}
	toolCallID := msg.ToolResult.ToolCallID

	for j := idx - 1; j >= 0; j-- {
		prev := messages[j]
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

func RenderMessageAt(p MessageRenderParams, idx int, isStreaming bool) string {
	msg := p.Messages[idx]
	var sb strings.Builder

	if msg.ToolResult == nil {
		sb.WriteString("\n")
	}

	switch msg.Role {
	case core.RoleUser:
		if msg.ToolResult != nil {
			sb.WriteString(RenderToolResultInline(ToolResultData{
				ToolName: msg.ToolName,
				Content:  msg.ToolResult.Content,
				Error:    msg.ToolResult.Content,
				IsError:  msg.ToolResult.IsError,
				Expanded: msg.Expanded,
			}, p.MDRenderer))
		} else {
			sb.WriteString(RenderUserMessage(msg.Content, msg.DisplayContent, msg.Images, p.MDRenderer, p.Width))
		}
	case core.RoleNotice:
		sb.WriteString(RenderSystemMessage(msg.Content))
	case core.RoleAssistant:
		sb.WriteString(renderAssistantWithTools(p, msg, idx, isStreaming))
	}

	return sb.String()
}

func renderAssistantWithTools(p MessageRenderParams, msg core.ChatMessage, idx int, isLast bool) string {
	base := RenderAssistantMessage(AssistantParams{
		Content:       msg.Content,
		Thinking:      msg.Thinking,
		ToolCalls:     msg.ToolCalls,
		StreamActive:  p.StreamActive,
		IsLast:        isLast,
		SpinnerView:   p.SpinnerView,
		MDRenderer:    p.MDRenderer,
		Width:         p.Width,
		ExecutingTool: p.BuildingTool,
	})

	if len(msg.ToolCalls) == 0 {
		return base
	}

	var sb strings.Builder
	sb.WriteString(base)

	if msg.Content != "" {
		sb.WriteString("\n")
	}

	resultMap := make(map[string]ToolResultData)
	for j := idx + 1; j < len(p.Messages); j++ {
		nextMsg := p.Messages[j]
		if nextMsg.Role == core.RoleNotice {
			continue
		}
		if nextMsg.ToolResult == nil {
			break
		}
		resultMap[nextMsg.ToolResult.ToolCallID] = ToolResultData{
			ToolName: nextMsg.ToolName,
			Content:  nextMsg.ToolResult.Content,
			Error:    nextMsg.ToolResult.Content,
			IsError:  nextMsg.ToolResult.IsError,
			Expanded: nextMsg.Expanded,
		}
	}

	sb.WriteString(RenderToolCalls(ToolCallsParams{
		ToolCalls:         msg.ToolCalls,
		ToolCallsExpanded: msg.ToolCallsExpanded,
		ResultMap:         resultMap,
		TaskProgress:      p.TaskProgress,
		SpinnerView:       p.SpinnerView,
		TaskOwnerMap:      p.TaskOwnerMap,
		MDRenderer:        p.MDRenderer,
		Width:             p.Width,
	}))

	return sb.String()
}

func RenderMessageRange(p MessageRenderParams, startIdx, endIdx int, includeSpinner bool) string {
	skipIndices := BuildSkipIndices(p.Messages, startIdx)
	var sb strings.Builder

	lastIdx := endIdx - 1
	isLastStreaming := p.StreamActive && lastIdx >= 0 && p.Messages[lastIdx].Role == core.RoleAssistant

	for i := startIdx; i < endIdx; i++ {
		if skipIndices[i] {
			continue
		}
		isStreaming := i == lastIdx && isLastStreaming
		sb.WriteString(RenderMessageAt(p, i, isStreaming))
	}

	if includeSpinner {
		sb.WriteString(renderPendingToolSpinnerFromParams(p, startIdx < endIdx))
	}

	return sb.String()
}

func RenderSingleMessage(p MessageRenderParams, idx int) string {
	if idx < 0 || idx >= len(p.Messages) {
		return ""
	}

	if p.Messages[idx].ToolResult != nil && IsToolResultInlined(p.Messages, idx) {
		return ""
	}

	return strings.TrimRight(RenderMessageAt(p, idx, false), "\n")
}

func RenderActiveContent(p MessageRenderParams) string {
	if p.CommittedCount >= len(p.Messages) {
		return renderPendingToolSpinnerFromParams(p, false)
	}
	return RenderMessageRange(p, p.CommittedCount, len(p.Messages), true)
}

func renderPendingToolSpinnerFromParams(p MessageRenderParams, suppressAgentLabel bool) string {
	return RenderPendingToolSpinner(PendingToolSpinnerParams{
		InteractivePromptActive: p.InteractivePromptActive,
		BuildingTool:            p.BuildingTool,
		TaskProgress:            p.TaskProgress,
		SpinnerView:             p.SpinnerView,
		Width:                   p.Width,
		SuppressAgentLabel:      suppressAgentLabel,
	})
}


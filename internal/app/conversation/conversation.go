package conversation

import (
	"github.com/yanmxa/gencode/internal/message"
)

// Append adds a message to the conversation.
func (m *Model) Append(msg message.ChatMessage) {
	m.Messages = append(m.Messages, msg)
}

// Clear resets the conversation to empty.
// Also used after compaction since the summary now lives in transcript state.
func (m *Model) Clear() {
	m.Messages = []message.ChatMessage{}
	m.CommittedCount = 0
}

// AddNotice appends a notice message to the conversation.
func (m *Model) AddNotice(content string) {
	m.Messages = append(m.Messages, message.ChatMessage{Role: message.RoleNotice, Content: content})
}

// AppendToLast appends text and thinking content to the last message.
func (m *Model) AppendToLast(text, thinking string) {
	if len(m.Messages) == 0 {
		return
	}
	idx := len(m.Messages) - 1
	if thinking != "" {
		m.Messages[idx].Thinking += thinking
	}
	if text != "" {
		m.Messages[idx].Content += text
	}
}

// SetLastToolCalls sets tool calls on the last message.
func (m *Model) SetLastToolCalls(calls []message.ToolCall) {
	if len(m.Messages) > 0 {
		m.Messages[len(m.Messages)-1].ToolCalls = calls
	}
}

// SetLastThinkingSignature sets the thinking signature on the last message.
func (m *Model) SetLastThinkingSignature(sig string) {
	if len(m.Messages) > 0 && sig != "" {
		m.Messages[len(m.Messages)-1].ThinkingSignature = sig
	}
}

// AppendErrorToLast appends an error to the last message content.
func (m *Model) AppendErrorToLast(err error) {
	if len(m.Messages) > 0 {
		idx := len(m.Messages) - 1
		m.Messages[idx].Content += "\n[Error: " + err.Error() + "]"
	}
}

// RemoveEmptyLastAssistant removes the last message if it's an empty assistant message.
func (m *Model) RemoveEmptyLastAssistant() {
	if len(m.Messages) > 0 {
		last := m.Messages[len(m.Messages)-1]
		if last.Role == message.RoleAssistant && last.Content == "" {
			m.Messages = m.Messages[:len(m.Messages)-1]
		}
	}
}

// MarkLastInterrupted marks the last assistant message as interrupted if it has no tool calls.
func (m *Model) MarkLastInterrupted() {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		msg := &m.Messages[i]
		if msg.Role != message.RoleAssistant {
			continue
		}
		if len(msg.ToolCalls) == 0 {
			if msg.Content == "" {
				msg.Content = "[Interrupted]"
			} else {
				msg.Content += " [Interrupted]"
			}
		}
		return
	}
}

// ToggleMostRecentExpandable toggles the expansion state of the most recent expandable message.
func (m *Model) ToggleMostRecentExpandable() {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		msg := &m.Messages[i]
		switch {
		case msg.ToolResult != nil:
			msg.Expanded = !msg.Expanded
			return
		case len(msg.ToolCalls) > 0:
			msg.ToolCallsExpanded = !msg.ToolCallsExpanded
			return
		}
	}
}

// HasAllToolResults checks if all tool calls at the given index have results.
func (m *Model) HasAllToolResults(idx int) bool {
	toolCalls := m.Messages[idx].ToolCalls
	if len(toolCalls) == 0 {
		return true
	}

	expected := make(map[string]bool, len(toolCalls))
	for _, tc := range toolCalls {
		expected[tc.ID] = false
	}

	for j := idx + 1; j < len(m.Messages); j++ {
		msg := m.Messages[j]
		if msg.Role == message.RoleNotice {
			continue
		}
		if msg.ToolResult == nil {
			break
		}
		if _, ok := expected[msg.ToolResult.ToolCallID]; ok {
			expected[msg.ToolResult.ToolCallID] = true
		}
		allFound := true
		for _, found := range expected {
			if !found {
				allFound = false
				break
			}
		}
		if allFound {
			return true
		}
	}

	return false
}

// ConvertToProvider converts chat messages to provider format, skipping notices.
func (m Model) ConvertToProvider() []message.Message {
	return m.ConvertToProviderFrom(0)
}

// ConvertToProviderFrom converts chat messages starting from startIdx to provider format.
func (m Model) ConvertToProviderFrom(startIdx int) []message.Message {
	if startIdx < 0 {
		startIdx = 0
	}
	providerMsgs := make([]message.Message, 0, len(m.Messages)-startIdx)
	for i := startIdx; i < len(m.Messages); i++ {
		msg := m.Messages[i]
		if msg.Role == message.RoleNotice {
			continue
		}

		providerMsg := message.Message{
			Role:              msg.Role,
			Content:           msg.Content,
			Images:            msg.Images,
			ToolCalls:         msg.ToolCalls,
			Thinking:          msg.Thinking,
			ThinkingSignature: msg.ThinkingSignature,
		}

		if msg.ToolResult != nil {
			tr := *msg.ToolResult
			if msg.ToolName != "" {
				tr.ToolName = msg.ToolName
			}
			providerMsg.ToolResult = &tr
		}

		providerMsgs = append(providerMsgs, providerMsg)
	}
	return providerMsgs
}

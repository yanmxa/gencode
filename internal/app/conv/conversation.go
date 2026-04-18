package conv

import (
	"github.com/yanmxa/gencode/internal/core"
)

// --- Stream state ---

// StreamState holds streaming-related display state for the TUI.
type StreamState struct {
	Active       bool
	BuildingTool string
}

// Stop clears streaming state.
func (s *StreamState) Stop() {
	s.Active = false
	s.BuildingTool = ""
}

// --- Conversation model ---

// ConversationModel holds the conversation message history, commit tracking,
// stream/compact state, modal prompts, and tool execution state.
type ConversationModel struct {
	Messages       []core.ChatMessage
	CommittedCount int
	Stream         StreamState
	Compact        CompactState
	Modal          ModalState
	Tool           ToolExecState
}

// NewConversation returns a fully initialized conversation model.
func NewConversation() ConversationModel {
	return ConversationModel{
		Messages: []core.ChatMessage{},
		Modal:    NewModalState(),
	}
}

// --- Conversation methods ---

// Append adds a message to the conversation.
func (m *ConversationModel) Append(msg core.ChatMessage) {
	m.Messages = append(m.Messages, msg)
}

// Clear resets the conversation to empty.
func (m *ConversationModel) Clear() {
	m.Messages = []core.ChatMessage{}
	m.CommittedCount = 0
}

// AddNotice appends a notice message to the conversation.
func (m *ConversationModel) AddNotice(content string) {
	m.Messages = append(m.Messages, core.ChatMessage{Role: core.RoleNotice, Content: content})
}

// AppendToLast appends text and thinking content to the last assistant message.
func (m *ConversationModel) AppendToLast(text, thinking string) {
	if len(m.Messages) == 0 {
		return
	}
	idx := len(m.Messages) - 1
	if m.Messages[idx].Role != core.RoleAssistant {
		return
	}
	if thinking != "" {
		m.Messages[idx].Thinking += thinking
	}
	if text != "" {
		m.Messages[idx].Content += text
	}
}

// SetLastToolCalls sets tool calls on the last message.
func (m *ConversationModel) SetLastToolCalls(calls []core.ToolCall) {
	if len(m.Messages) > 0 {
		m.Messages[len(m.Messages)-1].ToolCalls = calls
	}
}

// SetLastThinkingSignature sets the thinking signature on the last message.
func (m *ConversationModel) SetLastThinkingSignature(sig string) {
	if len(m.Messages) > 0 && sig != "" {
		m.Messages[len(m.Messages)-1].ThinkingSignature = sig
	}
}

// AppendErrorToLast appends an error to the last message content.
func (m *ConversationModel) AppendErrorToLast(err error) {
	if len(m.Messages) > 0 {
		idx := len(m.Messages) - 1
		m.Messages[idx].Content += "\n[Error: " + err.Error() + "]"
	}
}

// AppendCancelledToolResults appends error tool results for each cancelled tool call.
func (m *ConversationModel) AppendCancelledToolResults(calls []core.ToolCall, contentFn func(core.ToolCall) string) {
	for _, tc := range calls {
		m.Append(core.ChatMessage{
			Role:     core.RoleUser,
			ToolName: tc.Name,
			ToolResult: &core.ToolResult{
				ToolCallID: tc.ID,
				Content:    contentFn(tc),
				IsError:    true,
			},
		})
	}
}

// RemoveEmptyLastAssistant removes the last message if it's an empty assistant message.
func (m *ConversationModel) RemoveEmptyLastAssistant() {
	if len(m.Messages) > 0 {
		last := m.Messages[len(m.Messages)-1]
		if last.Role == core.RoleAssistant && last.Content == "" {
			m.Messages = m.Messages[:len(m.Messages)-1]
		}
	}
}

// MarkLastInterrupted marks the last assistant message as interrupted if it has no tool calls.
func (m *ConversationModel) MarkLastInterrupted() {
	for i := len(m.Messages) - 1; i >= 0; i-- {
		msg := &m.Messages[i]
		if msg.Role != core.RoleAssistant {
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
func (m *ConversationModel) ToggleMostRecentExpandable() {
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

// ToggleAllExpandable toggles expand/collapse for all tool results and tool calls.
// Returns true if any content was toggled (caller should reflow).
func (m *ConversationModel) ToggleAllExpandable() {
	anyExpanded := false
	for i := 0; i < len(m.Messages); i++ {
		msg := m.Messages[i]
		if (msg.ToolResult != nil && msg.Expanded) ||
			(len(msg.ToolCalls) > 0 && msg.ToolCallsExpanded) {
			anyExpanded = true
			break
		}
	}
	for i := 0; i < len(m.Messages); i++ {
		if m.Messages[i].ToolResult != nil {
			m.Messages[i].Expanded = !anyExpanded
		}
		if len(m.Messages[i].ToolCalls) > 0 {
			m.Messages[i].ToolCallsExpanded = !anyExpanded
		}
	}
}

// HasAllToolResults checks if all tool calls at the given index have results.
func (m *ConversationModel) HasAllToolResults(idx int) bool {
	if idx < 0 || idx >= len(m.Messages) {
		return true
	}
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
		if msg.Role == core.RoleNotice {
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
func (m ConversationModel) ConvertToProvider() []core.Message {
	return m.ConvertToProviderFrom(0)
}

// ConvertToProviderFrom converts chat messages starting from startIdx to provider format.
func (m ConversationModel) ConvertToProviderFrom(startIdx int) []core.Message {
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx > len(m.Messages) {
		startIdx = len(m.Messages)
	}
	providerMsgs := make([]core.Message, 0, len(m.Messages)-startIdx)
	for i := startIdx; i < len(m.Messages); i++ {
		msg := m.Messages[i]
		if msg.Role == core.RoleNotice {
			continue
		}

		providerMsg := core.Message{
			Role:              msg.Role,
			Content:           msg.Content,
			DisplayContent:    msg.DisplayContent,
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

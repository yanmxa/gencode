package conv

import "github.com/genai-io/san/internal/core"

// ChatMessage is the TUI projection of a core.Message plus transient render
// state. It belongs to the app layer; providers, agents, and persistence only
// exchange core.Message values.
type ChatMessage struct {
	ID                string
	Role              core.Role
	Content           string
	DisplayContent    string
	Thinking          string
	ThinkingSignature string
	Images            []core.Image
	ToolCalls         []core.ToolCall
	ToolResult        *core.ToolResult
	ToolCallsExpanded bool
	Expanded          bool

	ContentCommittedLen  int
	ThinkingCommittedLen int
	BulletEmitted        bool
	ThinkingEmitted      bool
}

func (m *ChatMessage) ResetStreamCommit() {
	m.ContentCommittedLen = 0
	m.ThinkingCommittedLen = 0
	m.BulletEmitted = false
	m.ThinkingEmitted = false
}

func (c ChatMessage) ToMessage() core.Message {
	msg := core.Message{
		ID:                c.ID,
		Role:              c.Role,
		Content:           c.Content,
		DisplayContent:    c.DisplayContent,
		Images:            c.Images,
		Thinking:          c.Thinking,
		ThinkingSignature: c.ThinkingSignature,
		ToolCalls:         c.ToolCalls,
	}
	if c.ToolResult != nil {
		tr := *c.ToolResult
		msg.ToolResult = &tr
	}
	return msg
}

func ChatFromMessage(m core.Message) ChatMessage {
	return ChatMessage{
		ID:                m.ID,
		Role:              m.Role,
		Content:           m.Content,
		DisplayContent:    m.DisplayContent,
		Images:            m.Images,
		Thinking:          m.Thinking,
		ThinkingSignature: m.ThinkingSignature,
		ToolCalls:         m.ToolCalls,
		ToolResult:        m.ToolResult,
	}
}

func ChatMessagesFromCore(messages []core.Message) []ChatMessage {
	result := make([]ChatMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, ChatFromMessage(message))
	}
	return result
}

func LastAssistantContent(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == core.RoleAssistant && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

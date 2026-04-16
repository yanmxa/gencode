package runtime

import (
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
)

func (l *Loop) lastAssistantContent() string {
	return core.LastAssistantContent(l.messages)
}

// Messages returns a copy of the current conversation history.
func (l *Loop) Messages() []core.Message {
	cp := make([]core.Message, len(l.messages))
	copy(cp, l.messages)
	return cp
}

// SetMessages replaces the conversation history. Used for session restore and forking.
// Makes a defensive copy so the caller cannot mutate the loop's internal state.
func (l *Loop) SetMessages(msgs []core.Message) {
	l.messages = append([]core.Message(nil), msgs...)
}

// Tokens returns the cumulative token usage tracked by the underlying client.
// Returns a zero value if no client is attached.
func (l *Loop) Tokens() provider.TokenUsage {
	if l.Client == nil {
		return provider.TokenUsage{}
	}
	return l.Client.Tokens()
}

// AddUser appends a user message (text + optional images) to the conversation.
func (l *Loop) AddUser(content string, images []core.Image) {
	l.messages = append(l.messages, core.UserMessage(content, images))
}

// AddResponse appends the assistant message, updates token counters, and returns tool calls.
func (l *Loop) AddResponse(resp *core.CompletionResponse) []core.ToolCall {
	if l.Client != nil {
		l.Client.AddUsage(resp.Usage)
	}

	l.messages = append(l.messages, core.AssistantMessage(resp.Content, resp.Thinking, resp.ToolCalls))

	return resp.ToolCalls
}

// AddToolResult appends a tool result message to the conversation.
func (l *Loop) AddToolResult(r core.ToolResult) {
	l.messages = append(l.messages, core.ToolResultMessage(r))
}

package conv

import (
	"testing"

	"github.com/genai-io/san/internal/core"
)

func TestAppendReturnsStableMessageIdentity(t *testing.T) {
	conversation := NewConversation()
	appended := conversation.Append(ChatMessage{Role: core.RoleUser, Content: "same text"})

	if appended.ID == "" {
		t.Fatal("Append returned an empty message ID")
	}
	if got := conversation.Messages[0].ID; got != appended.ID {
		t.Fatalf("stored ID = %q, returned ID = %q", got, appended.ID)
	}
	if got := appended.ToMessage().ID; got != appended.ID {
		t.Fatalf("provider message ID = %q, want %q", got, appended.ID)
	}
}

func TestApplyAgentAppendReconcilesAssistantAndToolResultIDs(t *testing.T) {
	conversation := NewConversation()
	conversation.Append(ChatMessage{Role: core.RoleAssistant, Content: "answer"})
	conversation.ApplyAgentAppend(core.Message{ID: "assistant-1", Role: core.RoleAssistant, Content: "answer"})
	if got := conversation.Messages[0].ID; got != "assistant-1" {
		t.Fatalf("assistant ID = %q, want assistant-1", got)
	}

	conversation.ApplyAgentAppend(core.Message{
		ID:   "tool-result-1",
		Role: core.RoleUser,
		ToolResult: &core.ToolResult{
			ToolCallID: "call-1",
			ToolName:   "Read",
		},
	})
	if got := conversation.takeToolMessageID("call-1"); got != "tool-result-1" {
		t.Fatalf("tool result ID = %q, want tool-result-1", got)
	}
	if got := conversation.takeToolMessageID("call-1"); got != "" {
		t.Fatalf("tool result ID was not consumed: %q", got)
	}
}

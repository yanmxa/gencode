package core

import (
	"strings"
	"testing"
)

func TestBuildConversationTextAggregatesToolCalls(t *testing.T) {
	text := BuildConversationText([]Message{
		AssistantMessage("", "", []ToolCall{
			{ID: "1", Name: "Bash"},
			{ID: "2", Name: "Bash"},
			{ID: "3", Name: "Glob"},
		}),
	})

	if !strings.Contains(text, "[Tool Calls: Bash × 2, Glob]") {
		t.Fatalf("BuildConversationText() = %q, want aggregated tool calls", text)
	}
	if strings.Count(text, "[Tool Call: Bash]") > 0 {
		t.Fatalf("BuildConversationText() = %q, should not emit repeated raw tool-call lines", text)
	}
}

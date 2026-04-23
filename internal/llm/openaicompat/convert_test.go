package openaicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
)

func TestConvertMessagesConvertsRoleToolResultToToolMessage(t *testing.T) {
	msgs := []core.Message{
		{
			Role: core.RoleAssistant,
			ToolCalls: []core.ToolCall{{
				ID:    "call_1",
				Name:  "Read",
				Input: `{"file_path":"README.md"}`,
			}},
		},
		{
			Role: core.RoleTool,
			ToolResult: &core.ToolResult{
				ToolCallID: "call_1",
				ToolName:   "Read",
				Content:    "ok",
			},
		},
	}

	converted := ConvertMessages(msgs, "", DefaultAssistantMessage)
	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal converted messages: %v", err)
	}
	got := string(raw)

	for _, want := range []string{
		`"role":"assistant"`,
		`"tool_calls"`,
		`"role":"tool"`,
		`"tool_call_id":"call_1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("converted messages missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"role":"system"`) {
		t.Fatalf("tool result must not be converted to system message:\n%s", got)
	}
}

func TestSanitizeToolMessagesStripsOrphanedAssistantToolCall(t *testing.T) {
	msgs := []core.Message{
		{
			Role: core.RoleAssistant,
			ToolCalls: []core.ToolCall{{
				ID:    "call_orphan",
				Name:  "Read",
				Input: `{}`,
			}},
		},
		{Role: core.RoleUser, Content: "continue"},
	}

	sanitized := SanitizeToolMessages(msgs)
	if len(sanitized) != 1 {
		t.Fatalf("expected orphaned empty assistant tool call to be dropped, got %d messages", len(sanitized))
	}
	if sanitized[0].Role != core.RoleUser || sanitized[0].Content != "continue" {
		t.Fatalf("unexpected sanitized messages: %#v", sanitized)
	}
}

func TestSanitizeToolMessagesKeepsOnlyImmediatelyAnsweredToolCalls(t *testing.T) {
	msgs := []core.Message{
		{
			Role:    core.RoleAssistant,
			Content: "checking",
			ToolCalls: []core.ToolCall{
				{ID: "call_1", Name: "Read", Input: `{}`},
				{ID: "call_2", Name: "Glob", Input: `{}`},
			},
		},
		{
			Role:       core.RoleTool,
			ToolResult: &core.ToolResult{ToolCallID: "call_1", ToolName: "Read", Content: "ok"},
		},
		{Role: core.RoleUser, Content: "next"},
	}

	sanitized := SanitizeToolMessages(msgs)
	if len(sanitized) != 3 {
		t.Fatalf("expected assistant, one tool result, and user message; got %d", len(sanitized))
	}
	if len(sanitized[0].ToolCalls) != 1 || sanitized[0].ToolCalls[0].ID != "call_1" {
		t.Fatalf("unexpected filtered tool calls: %#v", sanitized[0].ToolCalls)
	}
	if sanitized[1].ToolResult == nil || sanitized[1].ToolResult.ToolCallID != "call_1" {
		t.Fatalf("unexpected filtered tool result: %#v", sanitized[1])
	}
	if sanitized[2].Content != "next" {
		t.Fatalf("expected trailing user message to remain, got %#v", sanitized[2])
	}
}

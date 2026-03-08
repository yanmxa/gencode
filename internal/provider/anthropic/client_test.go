package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestToolIDSanitizer_ValidIDPassthrough(t *testing.T) {
	var s toolIDSanitizer
	id := "toolu_01ABC-xyz_99"
	if got := s.resolve(id); got != id {
		t.Errorf("resolve(%q) = %q, want passthrough", id, got)
	}
	if s.idMap != nil {
		t.Error("idMap should remain nil when all IDs are valid")
	}
}

func TestToolIDSanitizer_InvalidIDReplaced(t *testing.T) {
	var s toolIDSanitizer

	tests := []string{"call.abc.123", "fn:read:1", "id/with/slash", "has space"}
	for _, id := range tests {
		got := s.resolve(id)
		if !validToolIDPattern.MatchString(got) {
			t.Errorf("resolve(%q) = %q, not valid", id, got)
		}
	}
	if len(s.idMap) != len(tests) {
		t.Errorf("idMap has %d entries, want %d", len(s.idMap), len(tests))
	}
}

func TestToolIDSanitizer_StableMapping(t *testing.T) {
	var s toolIDSanitizer
	first := s.resolve("call.1")
	second := s.resolve("call.1")
	if first != second {
		t.Errorf("same input got different outputs: %q vs %q", first, second)
	}
}

func TestToolIDSanitizer_UniqueReplacements(t *testing.T) {
	var s toolIDSanitizer
	a := s.resolve("call.1")
	b := s.resolve("call.2")
	if a == b {
		t.Errorf("different inputs got same output: %q", a)
	}
}

func TestToolIDSanitizer_ConsistentAcrossToolUseAndResult(t *testing.T) {
	// Simulates the message conversion order: tool_use first, then tool_result
	var s toolIDSanitizer
	invalidID := "gemini.func.call/123"

	toolUseID := s.resolve(invalidID)
	toolResultID := s.resolve(invalidID)

	if toolUseID != toolResultID {
		t.Errorf("tool_use ID %q != tool_result ID %q", toolUseID, toolResultID)
	}
	if !validToolIDPattern.MatchString(toolUseID) {
		t.Errorf("resolved ID %q is not valid", toolUseID)
	}
}

func TestToolIDSanitizer_NoAllocationForValidIDs(t *testing.T) {
	var s toolIDSanitizer
	s.resolve("toolu_valid1")
	s.resolve("toolu_valid2")
	s.resolve("abc-def_123")

	if s.idMap != nil {
		t.Error("idMap should be nil when only valid IDs are resolved")
	}
}

func TestMergeConsecutiveMessages_ToolResults(t *testing.T) {
	// Simulate: assistant with 3 tool_use, followed by 3 separate user tool_result messages
	msgs := []anthropic.MessageParam{
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("Let me run these tools"),
			anthropic.NewToolUseBlock("tc_1", map[string]any{}, "read"),
			anthropic.NewToolUseBlock("tc_2", map[string]any{}, "read"),
			anthropic.NewToolUseBlock("tc_3", map[string]any{}, "read"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("tc_1", "result 1", false)),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("tc_2", "result 2", false)),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("tc_3", "result 3", false)),
	}

	merged := mergeConsecutiveMessages(msgs)

	if len(merged) != 2 {
		t.Fatalf("expected 2 messages after merge, got %d", len(merged))
	}
	if merged[0].Role != anthropic.MessageParamRoleAssistant {
		t.Errorf("first message should be assistant, got %s", merged[0].Role)
	}
	if merged[1].Role != anthropic.MessageParamRoleUser {
		t.Errorf("second message should be user, got %s", merged[1].Role)
	}
	if len(merged[1].Content) != 3 {
		t.Errorf("merged user message should have 3 content blocks, got %d", len(merged[1].Content))
	}
}

func TestMergeConsecutiveMessages_NoConsecutive(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("bye")),
	}

	merged := mergeConsecutiveMessages(msgs)
	if len(merged) != 3 {
		t.Fatalf("expected 3 messages (no merge needed), got %d", len(merged))
	}
}

func TestMergeConsecutiveMessages_Empty(t *testing.T) {
	merged := mergeConsecutiveMessages(nil)
	if len(merged) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(merged))
	}
}

func TestMergeConsecutiveMessages_Single(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
	}
	merged := mergeConsecutiveMessages(msgs)
	if len(merged) != 1 {
		t.Fatalf("expected 1 message, got %d", len(merged))
	}
}

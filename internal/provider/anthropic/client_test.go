package anthropic

import (
	"testing"
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

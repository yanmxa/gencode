package core

import "testing"

func TestEstimatePromptTokensUsesConversationGrowth(t *testing.T) {
	got := estimatePromptTokens(1000, 2000, 3000)
	if got != 1500 {
		t.Fatalf("estimatePromptTokens() = %d, want 1500", got)
	}
}

func TestEstimatePromptTokensNeverDropsBelowLastKnownPromptSize(t *testing.T) {
	got := estimatePromptTokens(1000, 3000, 2000)
	if got != 1000 {
		t.Fatalf("estimatePromptTokens() = %d, want 1000", got)
	}
}

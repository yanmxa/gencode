package app

import (
	"testing"

	"github.com/genai-io/san/internal/app/conv"
	"github.com/genai-io/san/internal/core"
)

func TestCountUserTurnsExcludesPendingByIdentityNotContent(t *testing.T) {
	messages := []conv.ChatMessage{
		{ID: "first", Role: core.RoleUser, Content: "repeat"},
		{ID: "assistant", Role: core.RoleAssistant, Content: "ok"},
		{ID: "pending", Role: core.RoleUser, Content: "repeat"},
	}

	if got := countUserTurns(messages, "pending"); got != 1 {
		t.Fatalf("countUserTurns(pending) = %d, want 1", got)
	}
	if got := countUserTurns(messages, "first"); got != 2 {
		t.Fatalf("countUserTurns(non-trailing ID) = %d, want 2", got)
	}
}

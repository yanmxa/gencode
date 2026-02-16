package compact_test

import (
	"context"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

// newFakeClient creates a *client.Client backed by the given responses.
func newFakeClient(responses ...message.CompletionResponse) (*client.Client, *client.FakeClient) {
	fake := &client.FakeClient{Responses: responses}
	return testutil.NewTestClient(fake), fake
}

func TestCompact_SummarizesConversation(t *testing.T) {
	c, _ := newFakeClient(
		message.CompletionResponse{Content: "Summary: discussed file reading", StopReason: "end_turn"},
	)

	msgs := []message.Message{
		message.UserMessage("read the file", nil),
		message.AssistantMessage("I'll read the file for you", "", nil),
		message.UserMessage("thanks", nil),
		message.AssistantMessage("you're welcome", "", nil),
	}

	summary, count, err := core.Compact(context.Background(), c, msgs, "")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}
	if count != 4 {
		t.Errorf("expected count 4, got %d", count)
	}
	if summary != "Summary: discussed file reading" {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestCompact_WithFocus(t *testing.T) {
	c, fake := newFakeClient(
		message.CompletionResponse{Content: "Focused summary on testing", StopReason: "end_turn"},
	)

	msgs := []message.Message{
		message.UserMessage("write tests", nil),
		message.AssistantMessage("ok", "", nil),
	}

	_, _, err := core.Compact(context.Background(), c, msgs, "testing")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}

	// Verify focus string appears in the messages sent to Complete
	if len(fake.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.Calls))
	}
	if !strings.Contains(fake.Calls[0].Messages[0].Content, "testing") {
		t.Error("expected focus string 'testing' in sent messages")
	}
}

func TestCompact_EmptyConversation(t *testing.T) {
	c, _ := newFakeClient(
		message.CompletionResponse{Content: "Empty summary", StopReason: "end_turn"},
	)

	summary, count, err := core.Compact(context.Background(), c, nil, "")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
	if summary == "" {
		t.Error("expected non-empty summary even for empty conversation")
	}
}

func TestNeedsCompaction(t *testing.T) {
	tests := []struct {
		name   string
		input  int
		limit  int
		expect bool
	}{
		{"zero limit", 100, 0, false},
		{"zero tokens", 0, 1000, false},
		{"well below", 500, 1000, false},
		{"at 94%", 940, 1000, false},
		{"at 95%", 950, 1000, true},
		{"at 100%", 1000, 1000, true},
		{"over limit", 1100, 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := message.NeedsCompaction(tt.input, tt.limit)
			if got != tt.expect {
				t.Errorf("NeedsCompaction(%d, %d) = %v, want %v",
					tt.input, tt.limit, got, tt.expect)
			}
		})
	}
}

package app

import (
	"strings"
	"testing"

	"github.com/genai-io/san/internal/app/conv"
	"github.com/genai-io/san/internal/app/hub"
	"github.com/genai-io/san/internal/app/trigger"
	"github.com/genai-io/san/internal/core"
	"github.com/genai-io/san/internal/subagent"
	"github.com/genai-io/san/internal/todo"
)

func TestInjectNotificationAppendsCanonicalAgentInput(t *testing.T) {
	m := newTurnQueueTestModel()
	m.injectNotification(hub.Message{Notice: "Task done", Content: "Use this result"})

	msg := lastUserMessage(t, m.conv.Messages)
	if msg.ID == "" || msg.Content != "Use this result" {
		t.Fatalf("notification input = %+v, want identified canonical message", msg)
	}
}

func TestInjectAsyncHookContinuationDeliversContextWithPrompt(t *testing.T) {
	m := newTurnQueueTestModel()
	m.injectAsyncHookContinuation(trigger.AsyncHookRewake{
		Context:            []string{"policy finding", "tool detail"},
		ContinuationPrompt: "Re-evaluate the plan",
	})

	msg := lastUserMessage(t, m.conv.Messages)
	for _, want := range []string{"policy finding", "tool detail", "Re-evaluate the plan"} {
		if !strings.Contains(msg.Content, want) {
			t.Fatalf("continuation input %q is missing %q", msg.Content, want)
		}
	}
	if msg.ID == "" {
		t.Fatal("continuation input has no stable message ID")
	}
}

func newTurnQueueTestModel() *model {
	return &model{
		env:  env{Width: 80},
		conv: conv.NewModel(80),
		services: services{
			Subagent: subagent.NewRegistry(),
			Plan:     todo.NewStore(),
		},
	}
}

func lastUserMessage(t *testing.T, messages []conv.ChatMessage) conv.ChatMessage {
	t.Helper()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == core.RoleUser {
			return messages[i]
		}
	}
	t.Fatal("no user message found")
	return conv.ChatMessage{}
}

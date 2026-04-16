package orchestration

import "testing"

func TestStoreQueuePendingMessageCreatesWorkerStub(t *testing.T) {
	store := newStore()

	if !store.QueuePendingMessage("task-1", "follow up later") {
		t.Fatal("expected queue to succeed")
	}
	if got := store.PendingMessageCount("task-1", ""); got != 1 {
		t.Fatalf("pending count = %d, want 1", got)
	}
}

func TestStoreDrainPendingMessagesByAgentID(t *testing.T) {
	store := newStore()
	store.RecordLaunch(Launch{
		TaskID:    "task-2",
		AgentID:   "agent-2",
		AgentType: "Explore",
	})
	store.QueuePendingMessage("task-2", "verify result")

	messages := store.DrainPendingMessages("", "agent-2")
	if len(messages) != 1 || messages[0] != "verify result" {
		t.Fatalf("messages = %#v", messages)
	}
	if got := store.PendingMessageCount("task-2", ""); got != 0 {
		t.Fatalf("pending count after drain = %d, want 0", got)
	}
}

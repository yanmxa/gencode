package app

import (
	"testing"

	"github.com/yanmxa/gencode/internal/core"
)

func TestDrainInputQueueRestoresQueuedInlineImages(t *testing.T) {
	m := newBaseModel(t.TempDir(), modelInfra{})
	label := m.userInput.AddPendingImage(core.Image{FileName: "queued.png"})
	m.inputQueue.Enqueue(label+" describe this", []core.Image{{FileName: "queued.png"}})
	m.userInput.ClearImages()
	m.userInput.Textarea.SetValue("")

	cmd := m.drainInputQueue()
	if cmd == nil {
		t.Fatal("expected drainInputQueue to start submission")
	}

	if m.inputQueue.Len() != 0 {
		t.Fatalf("expected queue to be drained, got %d items", m.inputQueue.Len())
	}
	if len(m.conv.Messages) == 0 {
		t.Fatal("expected conversation to include dequeued user message")
	}

	userMsg := m.conv.Messages[0]
	if userMsg.Role != core.RoleUser {
		t.Fatalf("expected first message to be user role, got %#v", userMsg)
	}
	if userMsg.Content != "describe this" {
		t.Fatalf("unexpected submitted content: %q", userMsg.Content)
	}
	if userMsg.DisplayContent != label+" describe this" {
		t.Fatalf("unexpected display content: %q", userMsg.DisplayContent)
	}
	if len(userMsg.Images) != 1 || userMsg.Images[0].FileName != "queued.png" {
		t.Fatalf("unexpected queued images: %#v", userMsg.Images)
	}
}

func TestSaveCurrentQueueEditPreservesImages(t *testing.T) {
	m := newBaseModel(t.TempDir(), modelInfra{})
	images := []core.Image{{FileName: "queued.png"}}
	m.inputQueue.Enqueue("[Image #1] prompt", images)
	m.queueSelectIdx = 0
	m.userInput.Textarea.SetValue("[Image #1] updated prompt")

	m.saveCurrentQueueEdit()

	item, ok := m.inputQueue.At(0)
	if !ok {
		t.Fatal("expected queue item to remain")
	}
	if item.Content != "[Image #1] updated prompt" {
		t.Fatalf("unexpected updated content: %q", item.Content)
	}
	if len(item.Images) != 1 || item.Images[0].FileName != "queued.png" {
		t.Fatalf("expected images to be preserved, got %#v", item.Images)
	}
}

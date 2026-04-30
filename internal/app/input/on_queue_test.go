package input

import (
	"testing"

	"github.com/yanmxa/gencode/internal/core"
)

func TestQueueEnqueueDequeue(t *testing.T) {
	var q Queue
	id := q.Enqueue("hello", nil)
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
	item, ok := q.Dequeue()
	if !ok || item.Content != "hello" {
		t.Fatalf("unexpected: %v, %v", item, ok)
	}
	_, ok = q.Dequeue()
	if ok {
		t.Fatal("expected empty")
	}
}

func TestQueueMaxSize(t *testing.T) {
	var q Queue
	for i := 0; i < maxQueueSize; i++ {
		q.Enqueue("item", nil)
	}
	if q.Enqueue("overflow", nil) != -1 {
		t.Fatal("expected -1")
	}
}

func TestQueueUpdateAtRemovesEmpty(t *testing.T) {
	var q Queue
	q.Enqueue("first", nil)
	q.Enqueue("second", nil)
	q.UpdateAt(0, "", nil)
	if q.Len() != 1 {
		t.Fatalf("expected 1, got %d", q.Len())
	}
	item, _ := q.At(0)
	if item.Content != "second" {
		t.Fatalf("expected 'second', got %q", item.Content)
	}
}

func TestQueueItems(t *testing.T) {
	var q Queue
	q.Enqueue("a", []core.Image{{FileName: "test.png"}})
	q.Enqueue("b", nil)
	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}
}

func TestQueuePendingItemsExcludeSentToInbox(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	q.Enqueue("b", nil)
	q.MarkSentToInbox(1)

	items := q.PendingItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 pending item, got %d", len(items))
	}
	if items[0].Content != "a" {
		t.Fatalf("expected pending item 'a', got %q", items[0].Content)
	}
	if q.PendingCount() != 1 {
		t.Fatalf("expected pending count 1, got %d", q.PendingCount())
	}
	if q.WaitingCount() != 1 {
		t.Fatalf("expected waiting count 1, got %d", q.WaitingCount())
	}
	if q.LastPendingIndex() != 0 {
		t.Fatalf("expected last pending index 0, got %d", q.LastPendingIndex())
	}
}

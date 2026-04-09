package queue

import (
	"testing"

	"github.com/yanmxa/gencode/internal/message"
)

func TestEnqueueDequeue(t *testing.T) {
	var q Queue

	id1 := q.Enqueue("first", nil)
	id2 := q.Enqueue("second", nil)

	if q.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", q.Len())
	}
	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}

	item, ok := q.Dequeue()
	if !ok || item.Content != "first" {
		t.Fatalf("expected first item, got %+v", item)
	}

	item, ok = q.Dequeue()
	if !ok || item.Content != "second" {
		t.Fatalf("expected second item, got %+v", item)
	}

	_, ok = q.Dequeue()
	if ok {
		t.Fatal("expected empty queue")
	}
}

func TestEnqueueWithImages(t *testing.T) {
	var q Queue

	images := []message.ImageData{{FileName: "test.png"}}
	q.Enqueue("with image", images)

	item, ok := q.Dequeue()
	if !ok {
		t.Fatal("expected item")
	}
	if len(item.Images) != 1 || item.Images[0].FileName != "test.png" {
		t.Fatalf("expected image data, got %+v", item.Images)
	}
}

func TestRemove(t *testing.T) {
	var q Queue

	q.Enqueue("a", nil)
	id2 := q.Enqueue("b", nil)
	q.Enqueue("c", nil)

	if !q.Remove(id2) {
		t.Fatal("expected Remove to return true")
	}
	if q.Len() != 2 {
		t.Fatalf("expected 2 items after remove, got %d", q.Len())
	}

	// Verify order: a, c
	item, _ := q.Dequeue()
	if item.Content != "a" {
		t.Fatalf("expected 'a', got %q", item.Content)
	}
	item, _ = q.Dequeue()
	if item.Content != "c" {
		t.Fatalf("expected 'c', got %q", item.Content)
	}
}

func TestRemoveNotFound(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	if q.Remove(999) {
		t.Fatal("expected Remove to return false for non-existent ID")
	}
}

func TestClear(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	q.Enqueue("b", nil)
	q.Clear()

	if q.Len() != 0 {
		t.Fatalf("expected empty queue after clear, got %d", q.Len())
	}
	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected empty dequeue after clear")
	}
}

func TestItems(t *testing.T) {
	var q Queue
	q.Enqueue("x", nil)
	q.Enqueue("y", nil)

	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Content != "x" || items[1].Content != "y" {
		t.Fatalf("unexpected items: %v", items)
	}
}

func TestAt(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	q.Enqueue("b", nil)

	item, ok := q.At(0)
	if !ok || item.Content != "a" {
		t.Fatalf("expected 'a' at index 0, got %+v", item)
	}
	item, ok = q.At(1)
	if !ok || item.Content != "b" {
		t.Fatalf("expected 'b' at index 1, got %+v", item)
	}
	_, ok = q.At(2)
	if ok {
		t.Fatal("expected false for out-of-bounds At")
	}
	_, ok = q.At(-1)
	if ok {
		t.Fatal("expected false for negative At")
	}
}

func TestMaxSize(t *testing.T) {
	var q Queue
	for i := 0; i < MaxSize; i++ {
		if id := q.Enqueue("x", nil); id < 0 {
			t.Fatalf("expected successful enqueue at %d", i)
		}
	}
	if id := q.Enqueue("overflow", nil); id != -1 {
		t.Fatal("expected -1 when queue is full")
	}
	if q.Len() != MaxSize {
		t.Fatalf("expected %d items, got %d", MaxSize, q.Len())
	}
}

func TestUpdateAt(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	q.Enqueue("b", nil)
	q.Enqueue("c", nil)

	// Update middle item in place
	q.UpdateAt(1, "B-edited", nil)
	items := q.Items()
	if items[0].Content != "a" || items[1].Content != "B-edited" || items[2].Content != "c" {
		t.Fatalf("expected [a, B-edited, c], got %v", items)
	}
}

func TestUpdateAtEmptyRemoves(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	q.Enqueue("b", nil)
	q.Enqueue("c", nil)

	// Clearing content removes the item
	q.UpdateAt(1, "", nil)
	if q.Len() != 2 {
		t.Fatalf("expected 2 items after empty update, got %d", q.Len())
	}
	items := q.Items()
	if items[0].Content != "a" || items[1].Content != "c" {
		t.Fatalf("expected [a, c], got %v", items)
	}
}

func TestUpdateAtOutOfBounds(t *testing.T) {
	var q Queue
	q.Enqueue("a", nil)
	if q.UpdateAt(-1, "x", nil) {
		t.Fatal("expected false for negative index")
	}
	if q.UpdateAt(5, "x", nil) {
		t.Fatal("expected false for out-of-bounds index")
	}
}

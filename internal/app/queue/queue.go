// Package queue provides an input queue for buffering user messages
// while the LLM is streaming or tools are executing.
package queue

import (
	"github.com/yanmxa/gencode/internal/message"
)

// MaxSize is the maximum number of items the queue will hold.
const MaxSize = 50

// Item represents a queued user input waiting to be processed.
type Item struct {
	ID      int
	Content string
	Images  []message.ImageData
}

// Queue stores pending user inputs in FIFO order.
type Queue struct {
	items  []Item
	nextID int
}

// Enqueue adds a new input to the end of the queue and returns its ID.
// Returns -1 if the queue is full.
func (q *Queue) Enqueue(content string, images []message.ImageData) int {
	if len(q.items) >= MaxSize {
		return -1
	}
	q.nextID++
	q.items = append(q.items, Item{
		ID:      q.nextID,
		Content: content,
		Images:  images,
	})
	return q.nextID
}

// Dequeue removes and returns the first item. Returns false if empty.
func (q *Queue) Dequeue() (Item, bool) {
	if len(q.items) == 0 {
		return Item{}, false
	}
	item := q.items[0]
	q.items[0] = Item{} // release references for GC
	q.items = q.items[1:]
	return item, true
}

// At returns the item at the given index without copying. Returns false if out of bounds.
func (q *Queue) At(idx int) (Item, bool) {
	if idx < 0 || idx >= len(q.items) {
		return Item{}, false
	}
	return q.items[idx], true
}

// Remove deletes the item with the given ID. Returns true if found.
func (q *Queue) Remove(id int) bool {
	for i, item := range q.items {
		if item.ID == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return true
		}
	}
	return false
}

// Items returns the internal slice directly. Callers must not modify it.
func (q *Queue) Items() []Item {
	return q.items
}

// Len returns the number of queued items.
func (q *Queue) Len() int {
	return len(q.items)
}

// UpdateAt updates the content of the item at the given index.
// If the content is empty and there are no images, the item is removed.
func (q *Queue) UpdateAt(idx int, content string, images []message.ImageData) bool {
	if idx < 0 || idx >= len(q.items) {
		return false
	}
	if content == "" && len(images) == 0 {
		q.items = append(q.items[:idx], q.items[idx+1:]...)
		return true
	}
	q.items[idx].Content = content
	q.items[idx].Images = images
	return true
}

// Clear removes all queued items.
func (q *Queue) Clear() {
	q.items = nil
}

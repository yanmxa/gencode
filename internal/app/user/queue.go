package user

import (
	"github.com/yanmxa/gencode/internal/core"
)

const maxQueueSize = 50

type QueueItem struct {
	ID      int
	Content string
	Images  []core.Image
}

type Queue struct {
	items  []QueueItem
	nextID int
}

func (q *Queue) Enqueue(content string, images []core.Image) int {
	if len(q.items) >= maxQueueSize {
		return -1
	}
	q.nextID++
	q.items = append(q.items, QueueItem{ID: q.nextID, Content: content, Images: images})
	return q.nextID
}

func (q *Queue) Dequeue() (QueueItem, bool) {
	if len(q.items) == 0 {
		return QueueItem{}, false
	}
	item := q.items[0]
	q.items[0] = QueueItem{}
	q.items = q.items[1:]
	return item, true
}

func (q *Queue) At(idx int) (QueueItem, bool) {
	if idx < 0 || idx >= len(q.items) {
		return QueueItem{}, false
	}
	return q.items[idx], true
}

func (q *Queue) Items() []QueueItem {
	out := make([]QueueItem, len(q.items))
	copy(out, q.items)
	return out
}

func (q *Queue) Len() int { return len(q.items) }

func (q *Queue) UpdateAt(idx int, content string, images []core.Image) bool {
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

func (q *Queue) Clear() { q.items = nil }

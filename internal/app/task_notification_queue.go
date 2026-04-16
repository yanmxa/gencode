package app

import (
	"sync"

	"github.com/yanmxa/gencode/internal/orchestration"
)

type taskNotification struct {
	Notice             string
	Context            []string
	ContinuationPrompt string
	Count              int
	TaskID             string
	Subject            string
	Status             string
	Batch              *orchestration.Batch
}

type taskNotificationQueue struct {
	mu    sync.Mutex
	items []taskNotification
}

func newTaskNotificationQueue() *taskNotificationQueue {
	return &taskNotificationQueue{}
}

func (q *taskNotificationQueue) Push(item taskNotification) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *taskNotificationQueue) Pop() (taskNotification, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return taskNotification{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *taskNotificationQueue) PopBatch(max int) []taskNotification {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 || max <= 0 {
		return nil
	}
	if len(q.items) < max {
		max = len(q.items)
	}
	items := append([]taskNotification(nil), q.items[:max]...)
	q.items = q.items[max:]
	return items
}

func (q *taskNotificationQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

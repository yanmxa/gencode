// Package notify handles Source 2 (background agent → main agent) inputs:
// task completion notifications and notification queue management.
package notify

import (
	"sync"

	"github.com/yanmxa/gencode/internal/orchestration"
)

type Model struct {
	Notifications *NotificationQueue
	BGTracker     *BackgroundTracker
}

func New() Model {
	return Model{
		Notifications: NewNotificationQueue(),
	}
}

type Notification struct {
	Notice             string
	Context            []string
	ContinuationPrompt string
	Count              int
	TaskID             string
	Subject            string
	Status             string
	Batch              *orchestration.Batch
}

// NotificationQueue is a thread-safe queue for task completion notifications.
type NotificationQueue struct {
	mu    sync.Mutex
	items []Notification
}

func NewNotificationQueue() *NotificationQueue {
	return &NotificationQueue{}
}

func (q *NotificationQueue) Push(item Notification) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *NotificationQueue) Pop() (Notification, bool) {
	if q == nil {
		return Notification{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Notification{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *NotificationQueue) PopBatch(max int) []Notification {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 || max <= 0 {
		return nil
	}
	if len(q.items) < max {
		max = len(q.items)
	}
	items := append([]Notification(nil), q.items[:max]...)
	q.items = q.items[max:]
	return items
}

func (q *NotificationQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

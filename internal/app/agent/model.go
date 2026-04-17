// Package agent handles Source 2 (agent → agent) inputs:
// background agent completion notifications, SendMessage, and self-inject.
package agent

import (
	"sync"

	"github.com/yanmxa/gencode/internal/orchestration"
)

// Model holds all agent-event input state: task notification queue and batch tracking.
type Model struct {
	Notifications *NotificationQueue
}

// New creates a Model with an initialized NotificationQueue.
func New() Model {
	return Model{
		Notifications: NewNotificationQueue(),
	}
}

// Notification holds data for a background task completion notification.
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

// NewNotificationQueue creates an initialized NotificationQueue.
func NewNotificationQueue() *NotificationQueue {
	return &NotificationQueue{}
}

// Push enqueues a notification.
func (q *NotificationQueue) Push(item Notification) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

// Pop dequeues the oldest notification.
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

// PopBatch dequeues up to max notifications at once.
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

// Len returns the number of queued notifications.
func (q *NotificationQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

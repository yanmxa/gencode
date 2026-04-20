// Package notify provides a generic message delivery mechanism between agents.
// Any agent can push a Message to a Queue; the receiver drains it when ready.
package notify

import "sync"

type Model struct {
	Queue     *Queue
	BGTracker *BackgroundTracker
}

func New() Model {
	return Model{
		Queue: NewQueue(),
	}
}

// Message is a notification delivered to an agent.
// Notice is displayed in the TUI; Content is sent to the LLM.
type Message struct {
	Notice  string
	Content string
}

// Queue is a thread-safe FIFO for delivering messages between agents.
type Queue struct {
	mu    sync.Mutex
	items []Message
}

func NewQueue() *Queue {
	return &Queue{}
}

func (q *Queue) Push(item Message) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *Queue) PopBatch(max int) []Message {
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
	items := append([]Message(nil), q.items[:max]...)
	q.items = q.items[max:]
	return items
}

func (q *Queue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

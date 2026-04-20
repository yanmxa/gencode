// Package hub provides a central EventHub for inter-agent communication.
// Producers call Publish(Event). The hub routes each event to the target's
// registered delivery function. No channels, no goroutines inside.
package hub

import (
	"sync"
	"time"
)

// Event is a CloudEvents-inspired message routed through the EventHub.
// It doubles as a tea.Msg for Bubble Tea Update dispatch.
type Event struct {
	Type    string    // "task.completed", "agent.message", "cron.fired"
	Source  string    // producer: "agent:<id>", "system:cron"
	Target  string   // consumer: "agent:<id>", "main"
	Subject string   // human-readable: "fix auth module completed"
	Data    string   // payload: XML content, message text, etc.
	Time    time.Time
}

// EventHub routes events to registered delivery functions.
// Each subscriber provides a func(Event) — the hub calls it directly.
type EventHub struct {
	mu   sync.RWMutex
	subs map[string]func(Event)
}

func New() *EventHub {
	return &EventHub{subs: make(map[string]func(Event))}
}

// Register adds a delivery function for the given subscriber ID.
func (h *EventHub) Register(id string, deliver func(Event)) {
	h.mu.Lock()
	h.subs[id] = deliver
	h.mu.Unlock()
}

// Unregister removes a subscriber's delivery function.
func (h *EventHub) Unregister(id string) {
	h.mu.Lock()
	delete(h.subs, id)
	h.mu.Unlock()
}

// Publish routes an event to the target's delivery function.
func (h *EventHub) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	h.mu.RLock()
	deliver, ok := h.subs[e.Target]
	h.mu.RUnlock()
	if ok {
		deliver(e)
	}
}

// Inbox is a thread-safe buffer for events awaiting consumption.
// Used by the TUI to collect events from background producers and
// drain them at turn boundaries.
type Inbox struct {
	mu    sync.Mutex
	items []Event
}

func (q *Inbox) Push(e Event) {
	q.mu.Lock()
	q.items = append(q.items, e)
	q.mu.Unlock()
}

// Drain returns up to max buffered events, removing them from the inbox.
func (q *Inbox) Drain(max int) []Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 || max <= 0 {
		return nil
	}
	if len(q.items) < max {
		max = len(q.items)
	}
	out := append([]Event(nil), q.items[:max]...)
	q.items = q.items[max:]
	return out
}

func (q *Inbox) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

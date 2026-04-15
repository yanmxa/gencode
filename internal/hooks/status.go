package hooks

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type activeHookStatus struct {
	Message string
	Seq     uint64
}

// StatusTracker tracks active hook status messages for display.
type StatusTracker struct {
	mu     sync.RWMutex
	seq    atomic.Uint64
	active map[string]activeHookStatus
}

// NewStatusTracker creates a new StatusTracker.
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		active: make(map[string]activeHookStatus),
	}
}

// Start begins tracking a status message and returns a key for later removal.
// Returns empty string if message is empty.
func (s *StatusTracker) Start(message string) string {
	if message == "" {
		return ""
	}
	seq := s.seq.Add(1)
	key := fmt.Sprintf("status-%d", seq)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[key] = activeHookStatus{
		Message: message,
		Seq:     seq,
	}
	return key
}

// End stops tracking a status message. No-op if id is empty.
func (s *StatusTracker) End(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, id)
}

// CurrentMessage returns the highest-sequence active status message.
func (s *StatusTracker) CurrentMessage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		message string
		maxSeq  uint64
	)
	for _, status := range s.active {
		if status.Seq >= maxSeq {
			maxSeq = status.Seq
			message = status.Message
		}
	}
	return message
}

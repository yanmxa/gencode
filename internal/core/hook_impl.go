package core

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// entry wraps a Hook with pre-compiled matcher.
type entry struct {
	Hook
	match func(string) bool // pre-compiled source matcher
	fired atomic.Bool        // ensures Once hooks fire at most once
}

// hooks is the default Hooks implementation.
type hooks struct {
	mu      sync.RWMutex
	byEvent map[EventType][]*entry
	seq     int64
	wg      sync.WaitGroup // tracks async hook goroutines
}

func NewHooks() Hooks {
	return &hooks{byEvent: make(map[EventType][]*entry)}
}

func (h *hooks) Register(hook Hook) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hook.ID == "" {
		h.seq++
		hook.ID = fmt.Sprintf("hook-%d", h.seq)
	}
	e := &entry{Hook: hook, match: compileMatcher(hook.Matcher)}
	h.byEvent[hook.Event] = append(h.byEvent[hook.Event], e)
	return hook.ID
}

func (h *hooks) Unregister(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.remove(id)
}

// remove deletes a hook by ID. Caller must hold mu write lock.
func (h *hooks) remove(id string) bool {
	for evt, list := range h.byEvent {
		for i, e := range list {
			if e.ID == id {
				h.byEvent[evt] = append(list[:i], list[i+1:]...)
				return true
			}
		}
	}
	return false
}

func (h *hooks) Fire(ctx context.Context, event Event) (Action, error) {
	// Snapshot matching entries under read lock, then release before calling handlers.
	h.mu.RLock()
	entries := h.byEvent[event.Type]
	if len(entries) == 0 {
		h.mu.RUnlock()
		return Action{}, nil
	}
	snapshot := make([]*entry, 0, len(entries))
	for _, e := range entries {
		if !e.match(event.Source) {
			continue
		}
		snapshot = append(snapshot, e)
	}
	h.mu.RUnlock()

	var merged Action
	for _, e := range snapshot {
		// Once hooks: use atomic CAS to guarantee at-most-once execution
		// even under concurrent Fire calls. The entry stays in the slice
		// with fired=true; subsequent snapshots skip it via the CAS check.
		// This avoids a deferred removal goroutine that could race with
		// re-registration of the same hook ID.
		if e.Once {
			if !e.fired.CompareAndSwap(false, true) {
				continue // another goroutine already fired this hook
			}
		}
		if e.Async {
			h.wg.Add(1)
			// Use a detached context so async hooks are not cancelled when the
			// caller's context expires (e.g., during agent shutdown). Drain()
			// provides the shutdown boundary for async hooks.
			asyncCtx := context.WithoutCancel(ctx)
			go func(handler Handler) {
				defer h.wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("core/hooks: async hook panicked: %v", r)
					}
				}()
				handler(asyncCtx, event)
			}(e.Handle)
			continue
		}
		action, err := e.Handle(ctx, event)
		if err != nil {
			return merged, err
		}
		merged = MergeActions(merged, action)
		if merged.Block {
			break
		}
	}

	return merged, nil
}

func (h *hooks) Has(event EventType) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byEvent[event]) > 0
}

// Drain blocks until all async hook goroutines have completed or the timeout
// expires. Call this during shutdown before closing channels that hooks may
// write to. A 10-second timeout prevents indefinite hangs from misbehaving
// async hooks that ignore context cancellation.
func (h *hooks) Drain() {
	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Printf("core/hooks: Drain timed out after 10s, some async hooks still running")
	}
}

// compileMatcher returns a match function for the given pattern.
func compileMatcher(pattern string) func(string) bool {
	if pattern == "" || pattern == "*" {
		return func(string) bool { return true }
	}
	if re, err := regexp.Compile("^(" + pattern + ")$"); err == nil {
		return re.MatchString
	}
	return func(v string) bool { return v == pattern }
}

// MergeActions combines two Actions following documented merge semantics.
func MergeActions(base, next Action) Action {
	if next.Block {
		base.Block = true
		base.Reason = next.Reason
	}
	if next.Modify != nil {
		base.Modify = next.Modify
	}
	if next.Inject != "" {
		if base.Inject != "" {
			base.Inject += "\n" + next.Inject
		} else {
			base.Inject = next.Inject
		}
	}
	if next.Meta != nil {
		if base.Meta == nil {
			base.Meta = make(map[string]any)
		}
		for k, v := range next.Meta {
			base.Meta[k] = v
		}
	}
	return base
}

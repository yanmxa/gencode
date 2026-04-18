// Package system handles Source 3 (system → agent) inputs:
// cron scheduled prompts and async hook rewakes.
package trigger

import (
	"sync"

	"github.com/yanmxa/gencode/internal/hook"
)

type Model struct {
	CronQueue      []string
	AsyncHookQueue *AsyncHookQueue
	HookStatus     string // temporary active hook status shown in status bar
	HookEngine     *hook.Engine
	FileWatcher    *FileWatcher
}

func New(hookEngine *hook.Engine) Model {
	return Model{
		AsyncHookQueue: NewAsyncHookQueue(),
		HookEngine:     hookEngine,
		FileWatcher:    NewFileWatcher(hookEngine, nil),
	}
}

func NewAsyncHookQueue() *AsyncHookQueue {
	return &AsyncHookQueue{}
}

// AsyncHookRewake holds data for an async hook continuation.
type AsyncHookRewake struct {
	Notice             string
	Context            []string
	ContinuationPrompt string
}

// AsyncHookQueue is a thread-safe queue for async hook rewake items.
type AsyncHookQueue struct {
	mu    sync.Mutex
	items []AsyncHookRewake
}

func (q *AsyncHookQueue) Push(item AsyncHookRewake) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *AsyncHookQueue) Pop() (AsyncHookRewake, bool) {
	if q == nil {
		return AsyncHookRewake{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return AsyncHookRewake{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *AsyncHookQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func RenderHookStatus(hookStatus, defaultModelName string) string {
	if hookStatus != "" {
		return hookStatus
	}
	return defaultModelName
}

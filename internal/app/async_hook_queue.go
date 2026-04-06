package app

import "sync"

type asyncHookRewake struct {
	Notice             string
	Context            []string
	ContinuationPrompt string
}

type asyncHookQueue struct {
	mu    sync.Mutex
	items []asyncHookRewake
}

func newAsyncHookQueue() *asyncHookQueue {
	return &asyncHookQueue{}
}

func (q *asyncHookQueue) Push(item asyncHookRewake) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *asyncHookQueue) Pop() (asyncHookRewake, bool) {
	if q == nil {
		return asyncHookRewake{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return asyncHookRewake{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *asyncHookQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

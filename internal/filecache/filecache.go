package filecache

import (
	"sort"
	"sync"
	"time"
)

const (
	MaxEntries       = 20
	RestoreMaxFiles  = 5
	RestoreMaxPerFile = 5000
	RestoreMaxTotal  = 50000
)

type Entry struct {
	FilePath  string
	Timestamp time.Time
}

type Cache struct {
	mu      sync.Mutex
	entries map[string]Entry
}

func New() *Cache {
	return &Cache{entries: make(map[string]Entry)}
}

func (c *Cache) Touch(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[filePath] = Entry{FilePath: filePath, Timestamp: time.Now()}
	c.evict()
}

func (c *Cache) Recent(n int) []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]Entry)
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *Cache) evict() {
	if len(c.entries) <= MaxEntries {
		return
	}
	entries := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	for i := 0; i < len(entries)-MaxEntries; i++ {
		delete(c.entries, entries[i].FilePath)
	}
}

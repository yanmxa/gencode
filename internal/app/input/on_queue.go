package input

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
)

const maxQueueSize = 50

type QueueItem struct {
	ID          int
	Content     string
	Images      []core.Image
	SentToInbox bool
}

type Queue struct {
	items     []QueueItem
	nextID    int
	SelectIdx int    // -1 = no selection, 0+ = selected queue item index
	Stashed   string // stashed textarea input while navigating into queue
}

func NewQueue() Queue {
	return Queue{SelectIdx: -1}
}

func (q *Queue) Enqueue(content string, images []core.Image) int {
	if len(q.items) >= maxQueueSize {
		return -1
	}
	q.nextID++
	q.items = append(q.items, QueueItem{ID: q.nextID, Content: content, Images: images})
	return q.nextID
}

func (q *Queue) Dequeue() (QueueItem, bool) {
	if len(q.items) == 0 {
		return QueueItem{}, false
	}
	item := q.items[0]
	q.items[0] = QueueItem{}
	q.items = q.items[1:]
	return item, true
}

func (q *Queue) At(idx int) (QueueItem, bool) {
	if idx < 0 || idx >= len(q.items) {
		return QueueItem{}, false
	}
	return q.items[idx], true
}

func (q *Queue) Items() []QueueItem {
	out := make([]QueueItem, len(q.items))
	copy(out, q.items)
	return out
}

func (q *Queue) PendingItems() []QueueItem {
	out := make([]QueueItem, 0, len(q.items))
	for _, item := range q.items {
		if !item.SentToInbox {
			out = append(out, item)
		}
	}
	return out
}

func (q *Queue) Len() int { return len(q.items) }

func (q *Queue) PendingCount() int {
	count := 0
	for _, item := range q.items {
		if !item.SentToInbox {
			count++
		}
	}
	return count
}

func (q *Queue) WaitingCount() int {
	count := 0
	for _, item := range q.items {
		if item.SentToInbox {
			count++
		}
	}
	return count
}

func (q *Queue) LastPendingIndex() int {
	for i := len(q.items) - 1; i >= 0; i-- {
		if !q.items[i].SentToInbox {
			return i
		}
	}
	return -1
}

func (q *Queue) UpdateAt(idx int, content string, images []core.Image) bool {
	if idx < 0 || idx >= len(q.items) {
		return false
	}
	if content == "" && len(images) == 0 {
		q.items = append(q.items[:idx], q.items[idx+1:]...)
		return true
	}
	q.items[idx].Content = content
	q.items[idx].Images = images
	return true
}

func (q *Queue) MarkSentToInbox(idx int) {
	if idx >= 0 && idx < len(q.items) {
		q.items[idx].SentToInbox = true
	}
}

func (q *Queue) Clear() { q.items = nil }

// HandleQueueSelectKey handles keys when a queue item is selected.
// Only Up, Down, Enter, and Escape are intercepted; all other keys pass
// through to the textarea for normal in-place editing.
func (m *Model) HandleQueueSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.Queue.SelectIdx < 0 {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.Queue.SelectIdx > 0 {
			m.SaveCurrentQueueEdit()
			qLen := m.Queue.Len()
			if qLen == 0 {
				m.ExitQueueSelection()
			} else {
				m.Queue.SelectIdx = min(m.Queue.SelectIdx, qLen) - 1
				m.LoadQueueItemIntoTextarea()
			}
		}
		return nil, true

	case tea.KeyDown:
		m.SaveCurrentQueueEdit()
		qLen := m.Queue.Len()
		if qLen == 0 || m.Queue.SelectIdx >= qLen-1 {
			m.ExitQueueSelection()
		} else {
			m.Queue.SelectIdx++
			m.LoadQueueItemIntoTextarea()
		}
		return nil, true

	case tea.KeyEnter, tea.KeyEsc:
		m.SaveCurrentQueueEdit()
		m.ExitQueueSelection()
		return nil, true
	}

	return nil, false
}

// EnterQueueSelection transitions into queue selection mode.
// Stashes current input and loads the last queue item into the textarea.
func (m *Model) EnterQueueSelection() {
	m.Queue.Stashed = m.Textarea.Value()
	m.Queue.SelectIdx = m.Queue.LastPendingIndex()
	if m.Queue.SelectIdx < 0 {
		m.Queue.Stashed = ""
		return
	}
	m.LoadQueueItemIntoTextarea()
}

// ExitQueueSelection leaves queue selection mode and restores stashed input.
func (m *Model) ExitQueueSelection() {
	m.Queue.SelectIdx = -1
	m.Textarea.SetValue(m.Queue.Stashed)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
	m.Queue.Stashed = ""
}

// SaveCurrentQueueEdit writes the current textarea content back to the
// selected queue item, preserving its position.
func (m *Model) SaveCurrentQueueEdit() {
	if m.Queue.SelectIdx < 0 || m.Queue.SelectIdx >= m.Queue.Len() {
		return
	}
	content := strings.TrimSpace(m.Textarea.Value())
	item, ok := m.Queue.At(m.Queue.SelectIdx)
	if !ok {
		return
	}
	m.Queue.UpdateAt(m.Queue.SelectIdx, content, item.Images)
}

// LoadQueueItemIntoTextarea loads the content of the selected queue item
// into the textarea for editing.
func (m *Model) LoadQueueItemIntoTextarea() {
	item, ok := m.Queue.At(m.Queue.SelectIdx)
	if !ok {
		return
	}
	if item.SentToInbox {
		m.ExitQueueSelection()
		return
	}
	m.Textarea.SetValue(item.Content)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

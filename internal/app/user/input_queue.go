package user

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appqueue "github.com/yanmxa/gencode/internal/app/user/queue"
)

// HandleQueueSelectKey handles keys when a queue item is selected.
// Only Up, Down, Enter, and Escape are intercepted; all other keys pass
// through to the textarea for normal in-place editing.
func (m *Model) HandleQueueSelectKey(q *appqueue.Queue, msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.QueueSelectIdx < 0 {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.QueueSelectIdx > 0 {
			m.SaveCurrentQueueEdit(q)
			qLen := q.Len()
			if qLen == 0 {
				m.ExitQueueSelection()
			} else {
				m.QueueSelectIdx = min(m.QueueSelectIdx, qLen) - 1
				m.LoadQueueItemIntoTextarea(q)
			}
		}
		return nil, true

	case tea.KeyDown:
		m.SaveCurrentQueueEdit(q)
		qLen := q.Len()
		if qLen == 0 || m.QueueSelectIdx >= qLen-1 {
			m.ExitQueueSelection()
		} else {
			m.QueueSelectIdx++
			m.LoadQueueItemIntoTextarea(q)
		}
		return nil, true

	case tea.KeyEnter, tea.KeyEsc:
		m.SaveCurrentQueueEdit(q)
		m.ExitQueueSelection()
		return nil, true
	}

	return nil, false
}

// EnterQueueSelection transitions into queue selection mode.
// Stashes current input and loads the last queue item into the textarea.
func (m *Model) EnterQueueSelection(q *appqueue.Queue) {
	m.QueueTempInput = m.Textarea.Value()
	m.QueueSelectIdx = q.Len() - 1
	m.LoadQueueItemIntoTextarea(q)
}

// ExitQueueSelection leaves queue selection mode and restores stashed input.
func (m *Model) ExitQueueSelection() {
	m.QueueSelectIdx = -1
	m.Textarea.SetValue(m.QueueTempInput)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
	m.QueueTempInput = ""
}

// SaveCurrentQueueEdit writes the current textarea content back to the
// selected queue item, preserving its position.
func (m *Model) SaveCurrentQueueEdit(q *appqueue.Queue) {
	if m.QueueSelectIdx < 0 || m.QueueSelectIdx >= q.Len() {
		return
	}
	content := strings.TrimSpace(m.Textarea.Value())
	item, ok := q.At(m.QueueSelectIdx)
	if !ok {
		return
	}
	q.UpdateAt(m.QueueSelectIdx, content, item.Images)
}

// LoadQueueItemIntoTextarea loads the content of the selected queue item
// into the textarea for editing.
func (m *Model) LoadQueueItemIntoTextarea(q *appqueue.Queue) {
	item, ok := q.At(m.QueueSelectIdx)
	if !ok {
		return
	}
	m.Textarea.SetValue(item.Content)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

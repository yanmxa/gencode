package user

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appqueue "github.com/yanmxa/gencode/internal/app/user/queue"
)

// HandleQueueSelectKey handles keys when a queue item is selected.
// Only Up, Down, Enter, and Escape are intercepted; all other keys pass
// through to the textarea for normal in-place editing.
func HandleQueueSelectKey(m *Model, q *appqueue.Queue, msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.QueueSelectIdx < 0 {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.QueueSelectIdx > 0 {
			SaveCurrentQueueEdit(m, q)
			qLen := q.Len()
			if qLen == 0 {
				ExitQueueSelection(m)
			} else {
				m.QueueSelectIdx = min(m.QueueSelectIdx, qLen) - 1
				LoadQueueItemIntoTextarea(m, q)
			}
		}
		return nil, true

	case tea.KeyDown:
		SaveCurrentQueueEdit(m, q)
		qLen := q.Len()
		if qLen == 0 || m.QueueSelectIdx >= qLen-1 {
			ExitQueueSelection(m)
		} else {
			m.QueueSelectIdx++
			LoadQueueItemIntoTextarea(m, q)
		}
		return nil, true

	case tea.KeyEnter, tea.KeyEsc:
		SaveCurrentQueueEdit(m, q)
		ExitQueueSelection(m)
		return nil, true
	}

	return nil, false
}

// EnterQueueSelection transitions into queue selection mode.
// Stashes current input and loads the last queue item into the textarea.
func EnterQueueSelection(m *Model, q *appqueue.Queue) {
	m.QueueTempInput = m.Textarea.Value()
	m.QueueSelectIdx = q.Len() - 1
	LoadQueueItemIntoTextarea(m, q)
}

// ExitQueueSelection leaves queue selection mode and restores stashed input.
func ExitQueueSelection(m *Model) {
	m.QueueSelectIdx = -1
	m.Textarea.SetValue(m.QueueTempInput)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
	m.QueueTempInput = ""
}

// SaveCurrentQueueEdit writes the current textarea content back to the
// selected queue item, preserving its position.
func SaveCurrentQueueEdit(m *Model, q *appqueue.Queue) {
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
func LoadQueueItemIntoTextarea(m *Model, q *appqueue.Queue) {
	item, ok := q.At(m.QueueSelectIdx)
	if !ok {
		return
	}
	m.Textarea.SetValue(item.Content)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

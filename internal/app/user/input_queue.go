package user

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// HandleQueueSelectKey handles keys when a queue item is selected.
// Only Up, Down, Enter, and Escape are intercepted; all other keys pass
// through to the textarea for normal in-place editing.
func (m *Model) HandleQueueSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.QueueSelectIdx < 0 {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.QueueSelectIdx > 0 {
			m.SaveCurrentQueueEdit()
			qLen := m.Queue.Len()
			if qLen == 0 {
				m.ExitQueueSelection()
			} else {
				m.QueueSelectIdx = min(m.QueueSelectIdx, qLen) - 1
				m.LoadQueueItemIntoTextarea()
			}
		}
		return nil, true

	case tea.KeyDown:
		m.SaveCurrentQueueEdit()
		qLen := m.Queue.Len()
		if qLen == 0 || m.QueueSelectIdx >= qLen-1 {
			m.ExitQueueSelection()
		} else {
			m.QueueSelectIdx++
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
	m.QueueTempInput = m.Textarea.Value()
	m.QueueSelectIdx = m.Queue.Len() - 1
	m.LoadQueueItemIntoTextarea()
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
func (m *Model) SaveCurrentQueueEdit() {
	if m.QueueSelectIdx < 0 || m.QueueSelectIdx >= m.Queue.Len() {
		return
	}
	content := strings.TrimSpace(m.Textarea.Value())
	item, ok := m.Queue.At(m.QueueSelectIdx)
	if !ok {
		return
	}
	m.Queue.UpdateAt(m.QueueSelectIdx, content, item.Images)
}

// LoadQueueItemIntoTextarea loads the content of the selected queue item
// into the textarea for editing.
func (m *Model) LoadQueueItemIntoTextarea() {
	item, ok := m.Queue.At(m.QueueSelectIdx)
	if !ok {
		return
	}
	m.Textarea.SetValue(item.Content)
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

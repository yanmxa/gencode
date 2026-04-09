package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleQueueSelectKey handles keys when a queue item is selected.
// Only Up, Down, Enter, and Escape are intercepted; all other keys pass
// through to the textarea for normal in-place editing.
func (m *model) handleQueueSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.queueSelectIdx < 0 {
		return nil, false
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.queueSelectIdx > 0 {
			m.saveCurrentQueueEdit()
			qLen := m.inputQueue.Len()
			if qLen == 0 {
				m.exitQueueSelection()
			} else {
				m.queueSelectIdx = min(m.queueSelectIdx, qLen) - 1
				m.loadQueueItemIntoTextarea()
			}
		}
		return nil, true

	case tea.KeyDown:
		m.saveCurrentQueueEdit()
		qLen := m.inputQueue.Len()
		if qLen == 0 || m.queueSelectIdx >= qLen-1 {
			m.exitQueueSelection()
		} else {
			m.queueSelectIdx++
			m.loadQueueItemIntoTextarea()
		}
		return nil, true

	case tea.KeyEnter, tea.KeyEsc:
		m.saveCurrentQueueEdit()
		m.exitQueueSelection()
		return nil, true
	}

	// All other keys (typing, left/right, backspace, etc.) pass through
	// to the textarea for normal editing of the selected queue item.
	return nil, false
}

// enterQueueSelection transitions into queue selection mode.
// Stashes current input and loads the last queue item into the textarea.
func (m *model) enterQueueSelection() {
	m.queueTempInput = m.input.Textarea.Value()
	m.queueSelectIdx = m.inputQueue.Len() - 1
	m.loadQueueItemIntoTextarea()
}

// exitQueueSelection leaves queue selection mode and restores stashed input.
func (m *model) exitQueueSelection() {
	m.queueSelectIdx = -1
	m.input.Textarea.SetValue(m.queueTempInput)
	m.input.Textarea.CursorEnd()
	m.input.UpdateHeight()
	m.queueTempInput = ""
}

// saveCurrentQueueEdit writes the current textarea content back to the
// selected queue item, preserving its position. If the content is empty,
// the item is removed from the queue.
func (m *model) saveCurrentQueueEdit() {
	if m.queueSelectIdx < 0 || m.queueSelectIdx >= m.inputQueue.Len() {
		return
	}
	content := strings.TrimSpace(m.input.Textarea.Value())
	item, ok := m.inputQueue.At(m.queueSelectIdx)
	if !ok {
		return
	}
	m.inputQueue.UpdateAt(m.queueSelectIdx, content, item.Images)
}

// loadQueueItemIntoTextarea loads the content of the selected queue item
// into the textarea for editing.
func (m *model) loadQueueItemIntoTextarea() {
	item, ok := m.inputQueue.At(m.queueSelectIdx)
	if !ok {
		return
	}
	m.input.Textarea.SetValue(item.Content)
	m.input.Textarea.CursorEnd()
	m.input.UpdateHeight()
}

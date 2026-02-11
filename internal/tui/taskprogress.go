package tui

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// TaskProgressMsg is sent when a Task tool reports progress
type TaskProgressMsg struct {
	Message string
}

// taskProgressChan is the global channel for task progress updates
var (
	taskProgressChan chan string
	taskProgressOnce sync.Once
)

// GetTaskProgressChan returns the global task progress channel
func GetTaskProgressChan() chan string {
	taskProgressOnce.Do(func() {
		taskProgressChan = make(chan string, 100)
	})
	return taskProgressChan
}

// SendTaskProgress sends a progress message (non-blocking)
func SendTaskProgress(msg string) {
	ch := GetTaskProgressChan()
	select {
	case ch <- msg:
	default:
		// Channel full, skip
	}
}

// checkTaskProgress returns a command that checks for task progress
func checkTaskProgress() tea.Cmd {
	return func() tea.Msg {
		ch := GetTaskProgressChan()
		select {
		case msg := <-ch:
			return TaskProgressMsg{Message: msg}
		default:
			return nil
		}
	}
}

// handleTaskProgress handles task progress messages
func (m *model) handleTaskProgress(msg TaskProgressMsg) (tea.Model, tea.Cmd) {
	// Add progress message to the list (keep last 5)
	m.taskProgress = append(m.taskProgress, msg.Message)
	if len(m.taskProgress) > 5 {
		m.taskProgress = m.taskProgress[1:]
	}

	// View() renders active content live, no viewport update needed
	// Continue checking for more progress
	return m, tea.Batch(m.spinner.Tick, checkTaskProgress())
}

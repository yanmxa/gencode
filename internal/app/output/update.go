package output

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/render"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/app/progress"
)

// drainProgress drains all pending task progress messages from the channel.
func (m *Model) drainProgress() {
	if m.ProgressHub == nil {
		return
	}
	m.TaskProgress = m.ProgressHub.Drain(m.TaskProgress)
}

// HandleProgress processes a task progress message.
func (m *Model) HandleProgress(msg progress.UpdateMsg) tea.Cmd {
	if m.TaskProgress == nil {
		m.TaskProgress = make(map[int][]string)
	}
	m.TaskProgress[msg.Index] = append(m.TaskProgress[msg.Index], msg.Message)
	// Cap progress entries per agent to prevent unbounded growth
	if len(m.TaskProgress[msg.Index]) > 5 {
		m.TaskProgress[msg.Index] = m.TaskProgress[msg.Index][len(m.TaskProgress[msg.Index])-5:]
	}

	if m.ProgressHub == nil {
		return m.Spinner.Tick
	}
	return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
}

// HandleProgressTick processes a progress tick when tasks may be running.
func (m *Model) HandleProgressTick(hasRunningTasks bool) tea.Cmd {
	if hasRunningTasks {
		if m.ProgressHub == nil {
			return m.Spinner.Tick
		}
		return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
	}
	return nil
}

// HandleTick handles spinner tick messages with context-aware updates.
func (m *Model) HandleTick(msg tea.Msg, active, fetching, compacting, interactiveActive, hasRunningTasks bool) tea.Cmd {
	// Handle token limit fetching spinner
	if fetching {
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return cmd
	}

	if !active && !hasRunningTasks {
		// Keep spinner alive when tasks are in-progress (e.g., background agents)
		if !tracker.DefaultStore.HasInProgress() {
			return nil
		}
	}

	var cmd tea.Cmd
	m.Spinner, cmd = m.Spinner.Update(msg)

	if interactiveActive {
		return cmd
	}

	// Check for Task progress updates (drains all pending messages)
	if hasRunningTasks {
		m.drainProgress()
	}

	return cmd
}

// ResizeMDRenderer recreates the markdown renderer for the given width.
func (m *Model) ResizeMDRenderer(width int) {
	m.MDRenderer = render.NewMDRenderer(width)
}

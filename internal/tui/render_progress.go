package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/tui/progress"
)

func (m *model) drainTaskProgress() {
	m.taskProgress = progress.Drain(m.taskProgress)
}

func (m *model) handleTaskProgress(msg progress.Msg) (tea.Model, tea.Cmd) {
	if m.taskProgress == nil {
		m.taskProgress = make(map[int][]string)
	}
	m.taskProgress[msg.Index] = append(m.taskProgress[msg.Index], msg.Message)
	if len(m.taskProgress[msg.Index]) > 5 {
		m.taskProgress[msg.Index] = m.taskProgress[msg.Index][1:]
	}

	return m, tea.Batch(m.spinner.Tick, progress.Check())
}

func (m *model) handleTaskProgressTick() (tea.Model, tea.Cmd) {
	if m.hasRunningTaskTools() {
		return m, tea.Batch(m.spinner.Tick, progress.Check())
	}
	return m, nil
}

func (m *model) hasRunningTaskTools() bool {
	if m.parallelMode {
		return m.hasRunningParallelTaskTools()
	}
	return m.hasRunningSequentialTaskTool()
}

// hasRunningParallelTaskTools checks for unfinished Task tools in parallel mode.
func (m *model) hasRunningParallelTaskTools() bool {
	for i, tc := range m.pendingToolCalls {
		if tc.Name == "Task" {
			if _, done := m.parallelResults[i]; !done {
				return true
			}
		}
	}
	return false
}

// hasRunningSequentialTaskTool checks if the current sequential tool is a Task.
func (m *model) hasRunningSequentialTaskTool() bool {
	if m.pendingToolCalls == nil || m.pendingToolIdx >= len(m.pendingToolCalls) {
		return false
	}
	return m.pendingToolCalls[m.pendingToolIdx].Name == "Task"
}

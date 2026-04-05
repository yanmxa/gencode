package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

func (m *model) handleTaskProgress(msg progress.UpdateMsg) tea.Cmd {
	return m.output.HandleProgress(msg)
}

func (m *model) handleTaskProgressTick() tea.Cmd {
	return m.output.HandleProgressTick(m.hasRunningTaskTools())
}

func (m *model) hasRunningTaskTools() bool {
	if m.tool.Parallel {
		return m.hasRunningParallelTaskTools()
	}
	return m.hasRunningSequentialTaskTool()
}

// hasRunningParallelTaskTools checks for unfinished Task tools in parallel mode.
func (m *model) hasRunningParallelTaskTools() bool {
	for i, tc := range m.tool.PendingCalls {
		if tc.Name == tool.ToolAgent {
			if _, done := m.tool.ParallelResults[i]; !done {
				return true
			}
		}
	}
	return false
}

// hasRunningSequentialTaskTool checks if the current sequential tool is a Task.
func (m *model) hasRunningSequentialTaskTool() bool {
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		return false
	}
	return m.tool.PendingCalls[m.tool.CurrentIdx].Name == tool.ToolAgent
}

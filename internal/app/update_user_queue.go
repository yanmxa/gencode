package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appuser "github.com/yanmxa/gencode/internal/app/user"
)

func (m *model) handleQueueSelectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	return appuser.HandleQueueSelectKey(&m.userInput, &m.inputQueue, msg)
}

func (m *model) enterQueueSelection() {
	appuser.EnterQueueSelection(&m.userInput, &m.inputQueue)
}

func (m *model) exitQueueSelection() {
	appuser.ExitQueueSelection(&m.userInput)
}

func (m *model) saveCurrentQueueEdit() {
	appuser.SaveCurrentQueueEdit(&m.userInput, &m.inputQueue)
}

func (m *model) loadQueueItemIntoTextarea() {
	appuser.LoadQueueItemIntoTextarea(&m.userInput, &m.inputQueue)
}

package searchui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/user/providerui"
)

type Runtime interface {
	SetProviderStatusMessage(msg string)
}

func Update(rt Runtime, state *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case SelectedMsg:
		state.Cancel()
		rt.SetProviderStatusMessage(fmt.Sprintf("Search engine: %s", msg.Provider))
		return providerui.StatusTimer(3 * time.Second), true
	}
	return nil, false
}

package app

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/providerui"
	"github.com/yanmxa/gencode/internal/app/searchui"
)

func (m *model) updateSearch(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case searchui.SelectedMsg:
		m.search.Selector.Cancel()
		m.provider.StatusMessage = fmt.Sprintf("Search engine: %s", msg.Provider)
		return providerui.StatusTimer(3 * time.Second), true
	}
	return nil, false
}

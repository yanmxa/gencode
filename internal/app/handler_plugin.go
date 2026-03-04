package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
)

// updatePlugin routes plugin management messages.
func (m *model) updatePlugin(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appplugin.EnableMsg:
		m.plugin.Selector.HandleEnable(msg.PluginName)
		return nil, true

	case appplugin.DisableMsg:
		m.plugin.Selector.HandleDisable(msg.PluginName)
		return nil, true

	case appplugin.UninstallMsg:
		m.plugin.Selector.HandleUninstall(msg.PluginName)
		return nil, true

	case appplugin.InstallMsg:
		return m.installPlugin(msg), true

	case appplugin.InstallResultMsg:
		m.plugin.Selector.HandleInstallResult(msg)
		if msg.Success {
			agent.Init(m.cwd)
		}
		return nil, true

	case appplugin.MarketplaceRemoveMsg:
		m.plugin.Selector.HandleMarketplaceRemove(msg.ID)
		return nil, true

	case appplugin.MarketplaceSyncResultMsg:
		m.plugin.Selector.HandleMarketplaceSync(msg)
		return nil, true
	}
	return nil, false
}

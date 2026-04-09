package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/pluginui"
	"github.com/yanmxa/gencode/internal/plugin"
)

// updatePlugin routes plugin management messages.
func (m *model) updatePlugin(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case pluginui.EnableMsg:
		m.plugin.Selector.HandleEnable(msg.PluginName)
		return nil, true

	case pluginui.DisableMsg:
		m.plugin.Selector.HandleDisable(msg.PluginName)
		return nil, true

	case pluginui.UninstallMsg:
		m.plugin.Selector.HandleUninstall(msg.PluginName)
		return nil, true

	case pluginui.InstallMsg:
		return m.installPlugin(msg), true

	case pluginui.InstallResultMsg:
		m.plugin.Selector.HandleInstallResult(msg)
		if msg.Success {
			_ = m.reloadPluginBackedState()
		}
		return nil, true

	case pluginui.MarketplaceRemoveMsg:
		m.plugin.Selector.HandleMarketplaceRemove(msg.ID)
		return nil, true

	case pluginui.MarketplaceSyncResultMsg:
		m.plugin.Selector.HandleMarketplaceSync(msg)
		return nil, true
	}
	return nil, false
}

// installPlugin creates a tea.Cmd that installs the requested plugin.
func (m model) installPlugin(msg pluginui.InstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(plugin.DefaultRegistry, m.cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return pluginui.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return pluginui.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return pluginui.InstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

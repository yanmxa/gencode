package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	"github.com/yanmxa/gencode/internal/plugin"
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

// installPlugin creates a tea.Cmd that installs the requested plugin.
func (m model) installPlugin(msg appplugin.InstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(plugin.DefaultRegistry, m.cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

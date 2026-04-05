package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	"github.com/yanmxa/gencode/internal/plugin"
)

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

package pluginui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/extension/plugin"
)

// Runtime defines the callbacks the pluginui package needs from the parent app model.
type Runtime interface {
	GetCwd() string
	ReloadPluginBackedState() error
}

// Update routes plugin management messages.
func Update(rt Runtime, state *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case EnableMsg:
		state.HandleEnable(msg.PluginName)
		return nil, true

	case DisableMsg:
		state.HandleDisable(msg.PluginName)
		return nil, true

	case UninstallMsg:
		state.HandleUninstall(msg.PluginName)
		return nil, true

	case InstallMsg:
		return installPlugin(state.registry, rt.GetCwd(), msg), true

	case InstallResultMsg:
		state.HandleInstallResult(msg)
		if msg.Success {
			_ = rt.ReloadPluginBackedState()
		}
		return nil, true

	case MarketplaceRemoveMsg:
		state.HandleMarketplaceRemove(msg.ID)
		return nil, true

	case MarketplaceSyncResultMsg:
		state.HandleMarketplaceSync(msg)
		return nil, true
	}
	return nil, false
}

// installPlugin creates a tea.Cmd that installs the requested plugin.
func installPlugin(reg *plugin.Registry, cwd string, msg InstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(reg, cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return InstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

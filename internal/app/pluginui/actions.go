package pluginui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
)

// buildInstalledActions returns actions for an installed plugin.
func (s *Model) buildInstalledActions(p PluginItem) []Action {
	actions := []Action{}
	if p.Enabled {
		actions = append(actions, Action{Label: "Disable plugin", Action: "disable"})
	} else {
		actions = append(actions, Action{Label: "Enable plugin", Action: "enable"})
	}
	actions = append(actions,
		Action{Label: "Uninstall", Action: "uninstall"},
		Action{Label: "Back to plugin list", Action: "back"},
	)
	return actions
}

// buildDiscoverActions returns actions for a discoverable plugin.
func (s *Model) buildDiscoverActions(p DiscoverPluginItem) []Action {
	actions := []Action{}
	if !p.Installed {
		actions = append(actions,
			Action{Label: "Install for you (user scope)", Action: "install_user"},
			Action{Label: "Install for all collaborators (project scope)", Action: "install_project"},
			Action{Label: "Install for you, in this repo only (local scope)", Action: "install_local"},
		)
	} else {
		actions = append(actions, Action{Label: "Already installed", Action: "none"})
	}
	if p.Homepage != "" {
		actions = append(actions, Action{Label: "Open homepage", Action: "homepage"})
	}
	actions = append(actions, Action{Label: "Back to plugin list", Action: "back"})
	return actions
}

// buildMarketplaceActions returns actions for a marketplace.
func (s *Model) buildMarketplaceActions(m MarketplaceItem) []Action {
	return []Action{
		{Label: fmt.Sprintf("Browse plugins (%d)", m.Available), Action: "browse"},
		{Label: "Update marketplace", Action: "update"},
		{Label: "Remove marketplace", Action: "remove"},
		{Label: "Back", Action: "back"},
	}
}

// executeAction executes the currently selected action.
func (s *Model) executeAction() tea.Cmd {
	if s.actionIdx >= len(s.actions) {
		return nil
	}
	action := s.actions[s.actionIdx]

	switch action.Action {
	case "enable":
		if s.detailPlugin != nil {
			return func() tea.Msg { return EnableMsg{PluginName: s.detailPlugin.FullName} }
		}
	case "disable":
		if s.detailPlugin != nil {
			return func() tea.Msg { return DisableMsg{PluginName: s.detailPlugin.FullName} }
		}
	case "uninstall":
		if s.detailPlugin != nil {
			return func() tea.Msg { return UninstallMsg{PluginName: s.detailPlugin.FullName} }
		}
	case "install_user":
		if s.detailDiscover != nil {
			return s.installPlugin(coreplugin.ScopeUser)
		}
	case "install_project":
		if s.detailDiscover != nil {
			return s.installPlugin(coreplugin.ScopeProject)
		}
	case "install_local":
		if s.detailDiscover != nil {
			return s.installPlugin(coreplugin.ScopeLocal)
		}
	case "homepage":
		if s.detailDiscover != nil && s.detailDiscover.Homepage != "" {
			s.setSuccess("Homepage: " + s.detailDiscover.Homepage)
		}
	case "browse":
		if s.detailMarketplace != nil {
			s.browseMarketplace()
		}
	case "update":
		if s.detailMarketplace != nil {
			return s.syncMarketplace(s.detailMarketplace.ID)
		}
	case "remove":
		if s.detailMarketplace != nil {
			return func() tea.Msg { return MarketplaceRemoveMsg{ID: s.detailMarketplace.ID} }
		}
	case "back":
		s.goBack()
	}
	return nil
}

// installPlugin creates an install command for the selected plugin.
func (s *Model) installPlugin(scope coreplugin.Scope) tea.Cmd {
	if s.detailDiscover == nil {
		return nil
	}
	name := s.detailDiscover.Name
	marketplace := s.detailDiscover.Marketplace
	s.isLoading = true
	s.loadingMsg = fmt.Sprintf("Installing %s...", name)
	return func() tea.Msg {
		return InstallMsg{
			PluginName:  name,
			Marketplace: marketplace,
			Scope:       scope,
		}
	}
}

// browseMarketplace enters the browse view for a marketplace.
func (s *Model) browseMarketplace() {
	if s.detailMarketplace == nil {
		return
	}

	s.browseMarketplaceID = s.detailMarketplace.ID
	s.browsePlugins = []DiscoverPluginItem{}

	plugins, err := s.marketplaceManager.ListPlugins(s.detailMarketplace.ID)
	if err != nil {
		s.setError(fmt.Sprintf("Failed to list plugins: %v", err))
		return
	}

	installedNames := s.getInstalledNames()
	for _, pluginName := range plugins {
		item := s.newDiscoverItem(pluginName, s.detailMarketplace.ID, installedNames)
		if pluginPath, err := s.marketplaceManager.GetPluginPath(s.detailMarketplace.ID, pluginName); err == nil {
			if p, err := coreplugin.LoadPlugin(pluginPath, coreplugin.ScopeUser, pluginName+"@"+s.detailMarketplace.ID); err == nil {
				item.Description = p.Manifest.Description
			}
		}
		s.browsePlugins = append(s.browsePlugins, item)
	}

	s.level = LevelBrowsePlugins
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// syncMarketplace creates a sync command for a marketplace.
func (s *Model) syncMarketplace(id string) tea.Cmd {
	s.isLoading = true
	s.loadingMsg = fmt.Sprintf("Syncing %s...", id)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := s.marketplaceManager.Sync(ctx, id); err != nil {
			return MarketplaceSyncResultMsg{ID: id, Success: false, Error: err}
		}
		return MarketplaceSyncResultMsg{ID: id, Success: true}
	}
}

// toggleSelectedPlugin toggles enable/disable for the selected plugin.
func (s *Model) toggleSelectedPlugin() tea.Cmd {
	if s.activeTab == TabInstalled && s.level == LevelTabList && s.selectedIdx < len(s.filteredItems) {
		if p, ok := s.filteredItems[s.selectedIdx].(PluginItem); ok {
			if p.Enabled {
				return func() tea.Msg { return DisableMsg{PluginName: p.FullName} }
			}
			return func() tea.Msg { return EnableMsg{PluginName: p.FullName} }
		}
	}
	return nil
}

// HandleEnable handles enabling a plugin.
func (s *Model) HandleEnable(name string) {
	if err := coreplugin.DefaultRegistry.Enable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to enable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Enabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleDisable handles disabling a plugin.
func (s *Model) HandleDisable(name string) {
	if err := coreplugin.DefaultRegistry.Disable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to disable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Disabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleUninstall handles uninstalling a plugin.
func (s *Model) HandleUninstall(name string) {
	if err := s.installer.Uninstall(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to uninstall: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Uninstalled %s", name))
		s.goBack()
	}
	s.refreshAndUpdateView()
}

// HandleInstallResult handles the result of plugin installation.
func (s *Model) HandleInstallResult(msg InstallResultMsg) {
	s.isLoading = false
	s.loadingMsg = ""
	if !msg.Success {
		s.setError(fmt.Sprintf("Failed to install: %v", msg.Error))
	} else {
		s.setSuccess(fmt.Sprintf("Installed %s", msg.PluginName))
		s.goBack()
	}
	s.refreshAndUpdateView()
}

// HandleMarketplaceSync handles marketplace sync result.
func (s *Model) HandleMarketplaceSync(msg MarketplaceSyncResultMsg) {
	s.isLoading = false
	s.loadingMsg = ""
	if !msg.Success {
		s.setError(fmt.Sprintf("Failed to sync %s: %v", msg.ID, msg.Error))
		if entry, ok := s.marketplaceManager.Get(msg.ID); ok && entry.Source.Source == "github" {
			if _, err := os.Stat(entry.InstallLocation); os.IsNotExist(err) {
				_ = s.marketplaceManager.Remove(msg.ID)
			}
		}
	} else {
		s.setSuccess(fmt.Sprintf("Synced %s", msg.ID))
	}
	s.refreshMarketplaces()
	s.refreshDiscoverPlugins()
}

// HandleMarketplaceRemove handles marketplace removal.
func (s *Model) HandleMarketplaceRemove(id string) {
	if err := s.marketplaceManager.Remove(id); err != nil {
		s.setError(fmt.Sprintf("Failed to remove: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Removed %s", id))
		s.goBack()
	}
	s.refreshMarketplaces()
}

// setError sets an error message.
func (s *Model) setError(msg string) {
	s.lastMessage = msg
	s.isError = true
}

// setSuccess sets a success message.
func (s *Model) setSuccess(msg string) {
	s.lastMessage = msg
	s.isError = false
}

// clearMessage clears the status message.
func (s *Model) clearMessage() {
	s.lastMessage = ""
	s.isError = false
}

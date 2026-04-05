package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
)

// EnterSelect enters plugin selection mode
func (s *Model) EnterSelect(width, height int) error {
	s.active = true
	s.width = width
	s.height = height
	s.clearMessage()
	s.searchQuery = ""
	s.level = LevelTabList
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.parentIdx = 0
	s.detailPlugin = nil
	s.detailDiscover = nil
	s.detailMarketplace = nil
	s.actions = nil
	s.actionIdx = 0
	s.addMarketplaceInput = ""
	s.browseMarketplaceID = ""

	availableLines := height - 13
	s.maxVisible = max(3, availableLines/3)

	if err := s.marketplaceManager.Load(); err != nil {
		s.setError(fmt.Sprintf("Failed to load marketplaces: %v", err))
	}
	_ = s.installer.LoadMarketplaces() // Non-fatal

	s.refreshCurrentTab()
	return nil
}

// refreshCurrentTab refreshes data for the current tab
func (s *Model) refreshCurrentTab() {
	switch s.activeTab {
	case TabInstalled:
		s.refreshInstalledPlugins()
	case TabDiscover:
		s.refreshDiscoverPlugins()
	case TabMarketplaces:
		s.refreshMarketplaces()
	}
	s.updateFilter()
}

// refreshInstalledPlugins loads installed plugins grouped by scope
func (s *Model) refreshInstalledPlugins() {
	plugins := coreplugin.DefaultRegistry.List()
	s.installedPlugins = make(map[coreplugin.Scope][]PluginItem)

	for _, p := range plugins {
		item := PluginItem{
			Name:        p.Manifest.Name,
			FullName:    p.FullName(),
			Description: p.Manifest.Description,
			Version:     p.Manifest.Version,
			Scope:       p.Scope,
			Enabled:     p.Enabled,
			Path:        p.Path,
			Skills:      len(p.Components.Skills),
			Agents:      len(p.Components.Agents),
			Commands:    len(p.Components.Commands),
			MCP:         len(p.Components.MCP),
			LSP:         len(p.Components.LSP),
			Errors:      p.Errors,
		}
		if p.Components.Hooks != nil {
			item.Hooks = len(p.Components.Hooks.Hooks)
		}
		if p.Manifest.Author != nil {
			item.Author = p.Manifest.Author.Name
		}
		item.Homepage = p.Manifest.Homepage

		if idx := strings.Index(p.Source, "@"); idx != -1 {
			item.Marketplace = p.Source[idx+1:]
		}

		s.installedPlugins[p.Scope] = append(s.installedPlugins[p.Scope], item)
	}

	for scope := range s.installedPlugins {
		sort.Slice(s.installedPlugins[scope], func(i, j int) bool {
			return s.installedPlugins[scope][i].Name < s.installedPlugins[scope][j].Name
		})
	}

	s.installedScopes = []coreplugin.Scope{}
	for _, scope := range []coreplugin.Scope{coreplugin.ScopeUser, coreplugin.ScopeProject, coreplugin.ScopeLocal, coreplugin.ScopeManaged} {
		if len(s.installedPlugins[scope]) > 0 {
			s.installedScopes = append(s.installedScopes, scope)
		}
	}

	s.installedFlatList = []PluginItem{}
	for _, scope := range s.installedScopes {
		s.installedFlatList = append(s.installedFlatList, s.installedPlugins[scope]...)
	}
}

// refreshDiscoverPlugins loads available plugins from all marketplaces
func (s *Model) refreshDiscoverPlugins() {
	s.discoverPlugins = []DiscoverPluginItem{}
	installedNames := s.getInstalledNames()

	for _, marketplaceID := range s.marketplaceManager.List() {
		plugins, err := s.marketplaceManager.ListPlugins(marketplaceID)
		if err != nil {
			continue
		}

		for _, pluginName := range plugins {
			item := s.newDiscoverItem(pluginName, marketplaceID, installedNames)
			s.enrichDiscoverItem(&item)
			s.discoverPlugins = append(s.discoverPlugins, item)
		}
	}

	sort.Slice(s.discoverPlugins, func(i, j int) bool {
		if s.discoverPlugins[i].Marketplace != s.discoverPlugins[j].Marketplace {
			return s.discoverPlugins[i].Marketplace < s.discoverPlugins[j].Marketplace
		}
		return s.discoverPlugins[i].Name < s.discoverPlugins[j].Name
	})
}

// refreshMarketplaces loads marketplace information
func (s *Model) refreshMarketplaces() {
	s.marketplaces = []MarketplaceItem{}

	installedCounts := make(map[string]int)
	for _, p := range coreplugin.DefaultRegistry.List() {
		if idx := strings.Index(p.Source, "@"); idx != -1 {
			marketplace := p.Source[idx+1:]
			installedCounts[marketplace]++
		}
	}

	for _, id := range s.marketplaceManager.List() {
		entry, ok := s.marketplaceManager.Get(id)
		if !ok {
			continue
		}

		item := MarketplaceItem{
			ID:         id,
			SourceType: entry.Source.Source,
			Installed:  installedCounts[id],
		}

		switch entry.Source.Source {
		case "github":
			item.Source = "https://github.com/" + entry.Source.Repo
		case "directory":
			item.Source = entry.Source.Path
		}

		if plugins, err := s.marketplaceManager.ListPlugins(id); err == nil {
			item.Available = len(plugins)
		}

		if entry.LastUpdated != "" {
			if t, err := time.Parse(time.RFC3339, entry.LastUpdated); err == nil {
				item.LastUpdated = t.Format("1/2/2006")
			}
		}

		item.IsOfficial = id == "claude-plugins-official"
		s.marketplaces = append(s.marketplaces, item)
	}

	sort.Slice(s.marketplaces, func(i, j int) bool {
		if s.marketplaces[i].IsOfficial != s.marketplaces[j].IsOfficial {
			return s.marketplaces[i].IsOfficial
		}
		return s.marketplaces[i].ID < s.marketplaces[j].ID
	})
}

// getInstalledNames returns a set of installed plugin names for quick lookup.
func (s *Model) getInstalledNames() map[string]bool {
	names := make(map[string]bool)
	for _, p := range coreplugin.DefaultRegistry.List() {
		names[p.FullName()] = true
		names[p.Name()] = true
	}
	return names
}

// newDiscoverItem creates a DiscoverPluginItem with installed status set.
func (s *Model) newDiscoverItem(name, marketplaceID string, installed map[string]bool) DiscoverPluginItem {
	fullName := name + "@" + marketplaceID
	return DiscoverPluginItem{
		Name:        name,
		Marketplace: marketplaceID,
		Installed:   installed[fullName] || installed[name],
	}
}

// enrichDiscoverItem loads manifest details into an item.
func (s *Model) enrichDiscoverItem(item *DiscoverPluginItem) {
	fullName := item.Name + "@" + item.Marketplace
	pluginPath, err := s.marketplaceManager.GetPluginPath(item.Marketplace, item.Name)
	if err != nil {
		return
	}
	p, err := coreplugin.LoadPlugin(pluginPath, coreplugin.ScopeUser, fullName)
	if err != nil {
		return
	}
	item.Description = p.Manifest.Description
	item.Version = p.Manifest.Version
	if p.Manifest.Author != nil {
		item.Author = p.Manifest.Author.Name
	}
	item.Homepage = p.Manifest.Homepage
}

// refreshAndUpdateView refreshes plugins and updates the detail view if active
func (s *Model) refreshAndUpdateView() {
	s.refreshCurrentTab()
	if s.level == LevelDetail && s.detailPlugin != nil {
		s.refreshDetailView()
	}
}

// refreshDetailView updates the detail plugin and actions after a state change
func (s *Model) refreshDetailView() {
	if s.detailPlugin == nil {
		return
	}
	name := s.detailPlugin.FullName
	for _, item := range s.filteredItems {
		if p, ok := item.(PluginItem); ok && p.FullName == name {
			s.detailPlugin = &p
			s.actions = s.buildInstalledActions(p)
			s.clampActionIdx()
			return
		}
	}
	s.goBack()
}

func (s *Model) clampActionIdx() {
	if s.actionIdx >= len(s.actions) {
		s.actionIdx = len(s.actions) - 1
	}
	if s.actionIdx < 0 {
		s.actionIdx = 0
	}
}

// addMarketplace adds a new marketplace
func (s *Model) addMarketplace() tea.Cmd {
	source := strings.TrimSpace(s.addMarketplaceInput)
	source = strings.TrimPrefix(source, "[")
	source = strings.TrimSuffix(source, "]")
	source = strings.TrimSpace(source)
	if source == "" {
		s.setError("Please enter a marketplace source")
		return nil
	}

	var id string
	var err error

	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		absPath := source
		if strings.HasPrefix(source, "~") {
			home, _ := os.UserHomeDir()
			absPath = filepath.Join(home, source[1:])
		}
		id = filepath.Base(absPath)
		err = s.marketplaceManager.AddDirectory(id, absPath)
	} else if strings.HasPrefix(source, "https://github.com/") {
		repo := strings.TrimPrefix(source, "https://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		repo = strings.TrimSuffix(repo, "/")
		parts := strings.Split(repo, "/")
		if len(parts) >= 2 {
			id = parts[len(parts)-1]
			err = s.marketplaceManager.AddGitHub(id, repo)
		} else {
			s.setError("Invalid GitHub URL format")
			return nil
		}
	} else if strings.Contains(source, "/") && !strings.Contains(source, "://") {
		parts := strings.Split(source, "/")
		id = parts[len(parts)-1]
		err = s.marketplaceManager.AddGitHub(id, source)
	} else {
		s.setError("Invalid source format. Use owner/repo, https://github.com/owner/repo, or ./path")
		return nil
	}

	if err != nil {
		s.setError(fmt.Sprintf("Failed to add marketplace: %v", err))
		return nil
	}

	s.level = LevelTabList
	s.addMarketplaceInput = ""
	s.refreshMarketplaces()

	return s.syncMarketplace(id)
}

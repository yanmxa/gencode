// Package plugin provides the plugin selector feature.
package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Tab represents the active tab in the plugin selector
type Tab int

const (
	TabDiscover     Tab = iota // Discover: browse marketplace plugins
	TabInstalled               // Default: installed plugins grouped by scope
	TabMarketplaces            // Marketplaces: manage marketplace sources
)

// Level represents the navigation level within the plugin selector
type Level int

const (
	LevelTabList        Level = iota // Tab's main list view
	LevelDetail                      // Plugin/marketplace detail view
	LevelInstallOptions              // Install scope selection
	LevelAddMarketplace              // Add marketplace dialog
	LevelBrowsePlugins               // Browse plugins in a marketplace
)

// Action represents an action available in detail view
type Action struct {
	Label  string
	Action string // "enable", "disable", "install", "uninstall", "update", "back", etc.
}

// PluginItem represents a plugin in the selector
type PluginItem struct {
	Name        string
	FullName    string
	Description string
	Version     string
	Scope       coreplugin.Scope
	Enabled     bool
	Path        string
	Skills      int
	Agents      int
	Commands    int
	Hooks       int
	MCP         int
	LSP         int
	Errors      []string
	Author      string
	Homepage    string
	Marketplace string // Source marketplace ID
}

// DiscoverPluginItem represents a plugin available in a marketplace
type DiscoverPluginItem struct {
	Name        string
	Description string
	Marketplace string
	Author      string
	Installed   bool
	Homepage    string
	Version     string
}

// MarketplaceItem represents a marketplace in the selector
type MarketplaceItem struct {
	ID          string
	Source      string // "owner/repo" or directory path
	SourceType  string // "github" or "directory"
	Available   int    // Number of available plugins
	Installed   int    // Number of installed plugins
	LastUpdated string
	IsOfficial  bool // Official marketplace indicator
}

// Model holds state for the plugin selector
type Model struct {
	active      bool
	width       int
	height      int
	lastMessage string // Status message (error or success)
	isError     bool   // true if lastMessage is an error
	maxVisible  int
	isLoading   bool   // true when async operation in progress
	loadingMsg  string // Message to show during loading (e.g., "Syncing...")

	// Tab navigation
	activeTab Tab

	// Installed Tab data
	installedPlugins  map[coreplugin.Scope][]PluginItem
	installedScopes   []coreplugin.Scope // Ordered scopes for display
	installedFlatList []PluginItem       // Flattened for navigation

	// Discover Tab data
	discoverPlugins []DiscoverPluginItem

	// Marketplaces Tab data
	marketplaces []MarketplaceItem

	// Current view state
	level        Level
	selectedIdx  int
	scrollOffset int

	// Search
	searchQuery   string
	filteredItems []any // Generic filtered items based on tab

	// Detail view
	detailPlugin      *PluginItem
	detailDiscover    *DiscoverPluginItem
	detailMarketplace *MarketplaceItem
	actions           []Action
	actionIdx         int
	parentIdx         int // Index when entering detail

	// Install options
	installScopes   []coreplugin.Scope
	installScopeIdx int

	// Add marketplace dialog
	addMarketplaceInput string
	addDialogCursor     int

	// Browse marketplace plugins
	browseMarketplaceID string
	browsePlugins       []DiscoverPluginItem

	// Managers
	marketplaceManager *coreplugin.MarketplaceManager
	installer          *coreplugin.Installer
}

// Plugin messages
type EnableMsg struct {
	PluginName string
}

type DisableMsg struct {
	PluginName string
}

type InstallMsg struct {
	PluginName  string
	Marketplace string
	Scope       coreplugin.Scope
}

type UninstallMsg struct {
	PluginName string
}

type InstallResultMsg struct {
	PluginName string
	Success    bool
	Error      error
}

type MarketplaceAddMsg struct {
	Source string
}

type MarketplaceRemoveMsg struct {
	ID string
}

type MarketplaceSyncMsg struct {
	ID string
}

type MarketplaceSyncResultMsg struct {
	ID      string
	Success bool
	Error   error
}


// New creates a new Model
func New() Model {
	cwd, _ := os.Getwd()
	return Model{
		active:             false,
		maxVisible:         15,           // Will be recalculated based on screen height
		activeTab:          TabInstalled, // Default to Installed tab
		installedPlugins:   make(map[coreplugin.Scope][]PluginItem),
		marketplaceManager: coreplugin.NewMarketplaceManager(cwd),
		installer:          coreplugin.NewInstaller(coreplugin.DefaultRegistry, cwd),
	}
}

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

	// Calculate maxVisible based on screen height
	// Each item takes ~3 lines (name, description, spacing)
	// Reserve space for: tabs (2), header (2), search (4), footer (3), padding (2)
	availableLines := height - 13
	s.maxVisible = max(3, availableLines/3)

	// Load marketplace manager
	if err := s.marketplaceManager.Load(); err != nil {
		s.setError(fmt.Sprintf("Failed to load marketplaces: %v", err))
	}
	if err := s.installer.LoadMarketplaces(); err != nil {
		// Non-fatal
	}

	// Refresh data for current tab
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

		// Extract marketplace from source (name@marketplace)
		if idx := strings.Index(p.Source, "@"); idx != -1 {
			item.Marketplace = p.Source[idx+1:]
		}

		s.installedPlugins[p.Scope] = append(s.installedPlugins[p.Scope], item)
	}

	// Sort plugins within each scope
	for scope := range s.installedPlugins {
		sort.Slice(s.installedPlugins[scope], func(i, j int) bool {
			return s.installedPlugins[scope][i].Name < s.installedPlugins[scope][j].Name
		})
	}

	// Build ordered scope list
	s.installedScopes = []coreplugin.Scope{}
	for _, scope := range []coreplugin.Scope{coreplugin.ScopeUser, coreplugin.ScopeProject, coreplugin.ScopeLocal, coreplugin.ScopeManaged} {
		if len(s.installedPlugins[scope]) > 0 {
			s.installedScopes = append(s.installedScopes, scope)
		}
	}

	// Build flat list for navigation
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

	// Sort by marketplace then name
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

	// Get installed plugin counts per marketplace
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

		// Get available plugin count
		if plugins, err := s.marketplaceManager.ListPlugins(id); err == nil {
			item.Available = len(plugins)
		}

		// Format last updated
		if entry.LastUpdated != "" {
			if t, err := time.Parse(time.RFC3339, entry.LastUpdated); err == nil {
				item.LastUpdated = t.Format("1/2/2006")
			}
		}

		// Mark official marketplaces
		item.IsOfficial = id == "claude-plugins-official"

		s.marketplaces = append(s.marketplaces, item)
	}

	// Sort: official first, then alphabetically
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

// enrichDiscoverItem loads manifest details (description, version, author, homepage) into an item.
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

// IsActive returns whether the selector is active
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *Model) Cancel() {
	s.active = false
	s.searchQuery = ""
	s.level = LevelTabList
	s.detailPlugin = nil
	s.detailDiscover = nil
	s.detailMarketplace = nil
	s.actions = nil
	s.actionIdx = 0
}

// Tab navigation
func (s *Model) NextTab() {
	s.switchTab((s.activeTab + 1) % 3)
}

func (s *Model) PrevTab() {
	s.switchTab((s.activeTab + 2) % 3)
}

func (s *Model) switchTab(tab Tab) {
	s.activeTab = tab
	s.level = LevelTabList
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.searchQuery = ""
	s.refreshCurrentTab()
}

// updateFilter filters items based on search query
func (s *Model) updateFilter() {
	query := strings.ToLower(s.searchQuery)
	s.filteredItems = s.filterItemsForTab(query)
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// filterItemsForTab returns filtered items based on the active tab and query
func (s *Model) filterItemsForTab(query string) []any {
	switch s.activeTab {
	case TabInstalled:
		return filterItems(s.installedFlatList, query, func(p PluginItem) []string {
			return []string{p.Name, p.Description}
		})
	case TabDiscover:
		return filterItems(s.discoverPlugins, query, func(p DiscoverPluginItem) []string {
			return []string{p.Name, p.Description, p.Marketplace}
		})
	case TabMarketplaces:
		return filterItems(s.marketplaces, query, func(m MarketplaceItem) []string {
			return []string{m.ID, m.Source}
		})
	default:
		return nil
	}
}

// filterItems is a generic filter function for any slice type
func filterItems[T any](items []T, query string, getFields func(T) []string) []any {
	if query == "" {
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result
	}

	result := make([]any, 0, len(items))
	for _, item := range items {
		for _, field := range getFields(item) {
			if shared.FuzzyMatch(strings.ToLower(field), query) {
				result = append(result, item)
				break
			}
		}
	}
	return result
}

// Navigation
func (s *Model) MoveUp() {
	s.clearMessage() // Clear status message on navigation
	switch s.level {
	case LevelDetail, LevelInstallOptions:
		if s.actionIdx > 0 {
			s.actionIdx--
		}
	default:
		if s.selectedIdx > 0 {
			s.selectedIdx--
			s.ensureVisible()
		}
	}
}

func (s *Model) MoveDown() {
	s.clearMessage() // Clear status message on navigation
	switch s.level {
	case LevelDetail, LevelInstallOptions:
		if s.actionIdx < len(s.actions)-1 {
			s.actionIdx++
		}
	default:
		maxIdx := s.getMaxIndex()
		if s.selectedIdx < maxIdx {
			s.selectedIdx++
			s.ensureVisible()
		}
	}
}

// getMaxIndex returns the maximum selectable index for the current view
func (s *Model) getMaxIndex() int {
	switch s.level {
	case LevelBrowsePlugins:
		return len(s.browsePlugins) - 1
	default:
		maxIdx := len(s.filteredItems) - 1
		if s.activeTab == TabMarketplaces {
			maxIdx++ // +1 for "Add Marketplace" item
		}
		return maxIdx
	}
}

func (s *Model) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// enterDetail enters the detail view for the selected item
func (s *Model) enterDetail() {
	s.parentIdx = s.selectedIdx

	switch s.activeTab {
	case TabInstalled:
		if s.selectedIdx >= len(s.filteredItems) {
			return
		}
		if p, ok := s.filteredItems[s.selectedIdx].(PluginItem); ok {
			s.detailPlugin = &p
			s.actions = s.buildInstalledActions(p)
			s.actionIdx = 0
			s.level = LevelDetail
		}

	case TabDiscover:
		if s.selectedIdx >= len(s.filteredItems) {
			return
		}
		if p, ok := s.filteredItems[s.selectedIdx].(DiscoverPluginItem); ok {
			s.detailDiscover = &p
			s.actions = s.buildDiscoverActions(p)
			s.actionIdx = 0
			s.level = LevelDetail
		}

	case TabMarketplaces:
		// Check if "Add Marketplace" is selected (first item)
		if s.selectedIdx == 0 {
			s.level = LevelAddMarketplace
			s.addMarketplaceInput = ""
			s.addDialogCursor = 0
			return
		}
		// Otherwise, select a marketplace
		mktIdx := s.selectedIdx - 1
		if mktIdx < len(s.filteredItems) {
			if m, ok := s.filteredItems[mktIdx].(MarketplaceItem); ok {
				s.detailMarketplace = &m
				s.actions = s.buildMarketplaceActions(m)
				s.actionIdx = 0
				s.level = LevelDetail
			}
		}
	}
}

// goBack returns to the previous view
func (s *Model) goBack() bool {
	switch s.level {
	case LevelDetail:
		s.level = LevelTabList
		s.selectedIdx = s.parentIdx
		s.detailPlugin = nil
		s.detailDiscover = nil
		s.detailMarketplace = nil
		s.actions = nil
		s.actionIdx = 0
		s.clearMessage()
		return true

	case LevelInstallOptions:
		// Go back to discover detail
		s.level = LevelDetail
		s.actions = s.buildDiscoverActions(*s.detailDiscover)
		s.actionIdx = 0
		return true

	case LevelAddMarketplace:
		s.level = LevelTabList
		s.addMarketplaceInput = ""
		return true

	case LevelBrowsePlugins:
		s.level = LevelDetail
		s.browsePlugins = nil
		s.browseMarketplaceID = ""
		s.selectedIdx = 0
		return true
	}
	return false
}

// buildInstalledActions returns actions for an installed plugin
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

// buildDiscoverActions returns actions for a discoverable plugin
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

// buildMarketplaceActions returns actions for a marketplace
func (s *Model) buildMarketplaceActions(m MarketplaceItem) []Action {
	return []Action{
		{Label: fmt.Sprintf("Browse plugins (%d)", m.Available), Action: "browse"},
		{Label: "Update marketplace", Action: "update"},
		{Label: "Remove marketplace", Action: "remove"},
		{Label: "Back", Action: "back"},
	}
}

// executeAction executes the currently selected action
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
		// Would open browser - for now just show message
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

// installPlugin creates an install command for the selected plugin
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

// browseMarketplace enters the browse view for a marketplace
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

// syncMarketplace creates a sync command for a marketplace
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

// addMarketplace adds a new marketplace
func (s *Model) addMarketplace() tea.Cmd {
	source := strings.TrimSpace(s.addMarketplaceInput)
	// Strip brackets that may come from terminal bracketed paste mode
	source = strings.TrimPrefix(source, "[")
	source = strings.TrimSuffix(source, "]")
	source = strings.TrimSpace(source)
	if source == "" {
		s.setError("Please enter a marketplace source")
		return nil
	}

	// Determine marketplace type and ID
	var id string
	var err error

	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		// Directory-based marketplace
		absPath := source
		if strings.HasPrefix(source, "~") {
			home, _ := os.UserHomeDir()
			absPath = filepath.Join(home, source[1:])
		}
		id = filepath.Base(absPath)
		err = s.marketplaceManager.AddDirectory(id, absPath)
	} else if strings.HasPrefix(source, "https://github.com/") {
		// Full GitHub URL: https://github.com/owner/repo
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
		// GitHub repo format: owner/repo
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

	// Sync the new marketplace
	s.level = LevelTabList
	s.addMarketplaceInput = ""
	s.refreshMarketplaces()

	return s.syncMarketplace(id)
}

// toggleSelectedPlugin toggles enable/disable for the selected plugin
func (s *Model) toggleSelectedPlugin() tea.Cmd {
	if s.activeTab == TabInstalled && s.level == LevelTabList {
		if s.selectedIdx < len(s.filteredItems) {
			if p, ok := s.filteredItems[s.selectedIdx].(PluginItem); ok {
				if p.Enabled {
					return func() tea.Msg { return DisableMsg{PluginName: p.FullName} }
				}
				return func() tea.Msg { return EnableMsg{PluginName: p.FullName} }
			}
		}
	}
	return nil
}

// HandleEnable handles enabling a plugin
func (s *Model) HandleEnable(name string) {
	if err := coreplugin.DefaultRegistry.Enable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to enable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Enabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleDisable handles disabling a plugin
func (s *Model) HandleDisable(name string) {
	if err := coreplugin.DefaultRegistry.Disable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to disable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Disabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleUninstall handles uninstalling a plugin
func (s *Model) HandleUninstall(name string) {
	if err := s.installer.Uninstall(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to uninstall: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Uninstalled %s", name))
		s.goBack()
	}
	s.refreshAndUpdateView()
}

// HandleInstallResult handles the result of plugin installation
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

// HandleMarketplaceSync handles marketplace sync result
func (s *Model) HandleMarketplaceSync(msg MarketplaceSyncResultMsg) {
	s.isLoading = false
	s.loadingMsg = ""
	if !msg.Success {
		s.setError(fmt.Sprintf("Failed to sync %s: %v", msg.ID, msg.Error))
		// Remove failed marketplace if it was never synced successfully
		// (InstallLocation directory doesn't exist)
		if entry, ok := s.marketplaceManager.Get(msg.ID); ok {
			if entry.Source.Source == "github" {
				if _, err := os.Stat(entry.InstallLocation); os.IsNotExist(err) {
					_ = s.marketplaceManager.Remove(msg.ID)
				}
			}
		}
	} else {
		s.setSuccess(fmt.Sprintf("Synced %s", msg.ID))
	}
	s.refreshMarketplaces()
	s.refreshDiscoverPlugins()
}

// HandleMarketplaceRemove handles marketplace removal
func (s *Model) HandleMarketplaceRemove(id string) {
	if err := s.marketplaceManager.Remove(id); err != nil {
		s.setError(fmt.Sprintf("Failed to remove: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Removed %s", id))
		s.goBack()
	}
	s.refreshMarketplaces()
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
	// Plugin no longer in list - go back
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

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Add marketplace dialog input
	if s.level == LevelAddMarketplace {
		return s.handleAddMarketplaceKeypress(key)
	}

	// Detail/install options view
	if s.level == LevelDetail || s.level == LevelInstallOptions {
		return s.handleDetailKeypress(key)
	}

	// Browse plugins view
	if s.level == LevelBrowsePlugins {
		return s.handleBrowseKeypress(key)
	}

	// Tab list view
	return s.handleListKeypress(key)
}

func (s *Model) handleAddMarketplaceKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEsc:
		s.goBack()
		return nil
	case tea.KeyEnter:
		return s.addMarketplace()
	case tea.KeyBackspace:
		if len(s.addMarketplaceInput) > 0 {
			s.addMarketplaceInput = s.addMarketplaceInput[:len(s.addMarketplaceInput)-1]
		}
		return nil
	case tea.KeyRunes:
		input := key.String()
		// Strip brackets from bracketed paste mode escape sequences
		// These can come as: "[content]" or separate events "[", "content", "]"
		if s.addMarketplaceInput == "" {
			// At start, strip leading bracket
			input = strings.TrimPrefix(input, "[")
		}
		// Always strip trailing bracket (not valid in paths or owner/repo)
		input = strings.TrimSuffix(input, "]")
		if input != "" {
			s.addMarketplaceInput += input
		}
		return nil
	}
	return nil
}

func (s *Model) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
	if s.handleNavigationKey(key) {
		return nil
	}
	switch key.Type {
	case tea.KeyEnter:
		return s.executeAction()
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
	case tea.KeyRunes:
		if key.String() == "h" {
			s.goBack()
		}
	}
	return nil
}

func (s *Model) handleBrowseKeypress(key tea.KeyMsg) tea.Cmd {
	if s.handleNavigationKey(key) {
		return nil
	}
	switch key.Type {
	case tea.KeyEnter:
		if s.selectedIdx < len(s.browsePlugins) {
			p := s.browsePlugins[s.selectedIdx]
			s.detailDiscover = &p
			s.actions = s.buildDiscoverActions(p)
			s.actionIdx = 0
			s.level = LevelDetail
		}
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
	}
	return nil
}

// handleNavigationKey handles common up/down navigation keys, returns true if handled
func (s *Model) handleNavigationKey(key tea.KeyMsg) bool {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return true
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return true
	case tea.KeyRunes:
		switch key.String() {
		case "k":
			s.MoveUp()
			return true
		case "j":
			s.MoveDown()
			return true
		}
	}
	return false
}

func (s *Model) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	// Handle tab navigation when not searching
	if s.searchQuery == "" {
		switch key.Type {
		case tea.KeyTab, tea.KeyRight:
			s.NextTab()
			return nil
		case tea.KeyShiftTab, tea.KeyLeft:
			s.PrevTab()
			return nil
		}
	}

	// Handle common navigation keys
	if s.handleNavigationKey(key) {
		return nil
	}

	switch key.Type {
	case tea.KeyEnter:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return shared.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		return s.handleListRuneKey(key.String())
	}
	return nil
}

// handleListRuneKey handles rune key input in list view
func (s *Model) handleListRuneKey(r string) tea.Cmd {
	if s.searchQuery == "" {
		switch r {
		case "l":
			s.enterDetail()
			return nil
		case " ":
			return s.toggleSelectedPlugin()
		case "u":
			return s.handleMarketplaceAction(func(m MarketplaceItem) tea.Cmd {
				return s.syncMarketplace(m.ID)
			})
		case "r":
			return s.handleMarketplaceAction(func(m MarketplaceItem) tea.Cmd {
				return func() tea.Msg { return MarketplaceRemoveMsg{ID: m.ID} }
			})
		}
	}
	s.searchQuery += r
	s.updateFilter()
	return nil
}

// handleMarketplaceAction executes an action on the selected marketplace
func (s *Model) handleMarketplaceAction(action func(MarketplaceItem) tea.Cmd) tea.Cmd {
	if s.activeTab != TabMarketplaces || s.selectedIdx == 0 {
		return nil
	}
	mktIdx := s.selectedIdx - 1
	if mktIdx < len(s.filteredItems) {
		if m, ok := s.filteredItems[mktIdx].(MarketplaceItem); ok {
			return action(m)
		}
	}
	return nil
}

// Render renders the plugin selector
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	switch s.level {
	case LevelDetail:
		if s.detailPlugin != nil {
			return s.renderInstalledDetail()
		}
		if s.detailDiscover != nil {
			return s.renderDiscoverDetail()
		}
		if s.detailMarketplace != nil {
			return s.renderMarketplaceDetail()
		}
	case LevelAddMarketplace:
		return s.renderAddMarketplaceDialog()
	case LevelBrowsePlugins:
		return s.renderBrowsePlugins()
	}

	// Tab list view
	return s.renderTabList()
}

// renderTabs renders the tab navigation bar like Claude Code
func (s *Model) renderTabs() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Bold(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)
	separatorStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim)

	tabs := []struct {
		name string
		tab  Tab
	}{
		{"Discover", TabDiscover},
		{"Installed", TabInstalled},
		{"Marketplaces", TabMarketplaces},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == s.activeTab {
			parts = append(parts, activeStyle.Render(t.name))
		} else {
			parts = append(parts, inactiveStyle.Render(t.name))
		}
	}

	return strings.Join(parts, separatorStyle.Render("  |  "))
}

// renderTabList renders the main tab list view
func (s *Model) renderTabList() string {
	var sb strings.Builder

	// Tab bar (centered)
	tabBar := s.renderTabs()
	sb.WriteString(tabBar)
	sb.WriteString("\n\n")

	// Search box
	s.renderSearchBox(&sb)
	sb.WriteString("\n\n")

	// Content based on tab
	switch s.activeTab {
	case TabInstalled:
		s.renderInstalledList(&sb)
	case TabDiscover:
		s.renderDiscoverList(&sb)
	case TabMarketplaces:
		s.renderMarketplacesList(&sb)
	}

	// Footer
	hint := s.getTabHint()
	s.renderFooter(&sb, hint)
	return s.renderBox(sb.String())
}

// getItemCount returns current position and total count for the active tab
func (s *Model) getItemCount() (int, int) {
	total := len(s.filteredItems)
	if s.activeTab == TabMarketplaces {
		total++ // +1 for "Add Marketplace"
	}
	pos := s.selectedIdx + 1
	if total == 0 {
		pos = 0
	}
	return pos, total
}

// renderSearchBox renders the search input
func (s *Model) renderSearchBox(sb *strings.Builder) {
	searchStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	inputStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	pos, total := s.getItemCount()
	countText := fmt.Sprintf("  %d/%d", pos, total)

	if s.searchQuery == "" {
		sb.WriteString(searchStyle.Render("\u2315 Search..."))
		sb.WriteString(searchStyle.Render(countText))
	} else {
		sb.WriteString(searchStyle.Render("\u2315 "))
		sb.WriteString(inputStyle.Render(s.searchQuery))
		sb.WriteString(inputStyle.Render("\u2502"))
		sb.WriteString(searchStyle.Render(countText))
	}
}

func (s *Model) getTabHint() string {
	switch s.activeTab {
	case TabInstalled:
		return "\u2191\u2193 navigate \u00b7 space toggle \u00b7 enter details \u00b7 esc close"
	case TabDiscover:
		return "\u2191\u2193 navigate \u00b7 enter details \u00b7 esc close"
	case TabMarketplaces:
		return "\u2191\u2193 navigate \u00b7 u update \u00b7 r remove \u00b7 esc close"
	}
	return ""
}

func (s *Model) renderInstalledList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.installedFlatList) == 0 {
			sb.WriteString(dimStyle.Render("No plugins installed"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Run: gen plugin install <name>@<marketplace>"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("No matches"))
			sb.WriteString("\n")
		}
		return
	}

	endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(PluginItem)
		if !ok {
			continue
		}

		icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(p.Name)

		if p.Marketplace != "" {
			sb.WriteString(dimStyle.Render(" \u00b7 " + p.Marketplace))
		}

		// Description inline after marketplace
		if p.Description != "" {
			prefixLen := 4 + len(p.Name) + 3 + len(p.Marketplace) // cursor + icon + name + " . " + marketplace
			maxDescLen := s.width - prefixLen - 5
			if maxDescLen > 20 {
				desc := shared.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.Render(" \u00b7 " + desc))
			}
		}
		sb.WriteString("\n")
	}

	// Scroll indicator
	if s.scrollOffset > 0 || endIdx < len(s.filteredItems) {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  (%d more)", len(s.filteredItems)-s.maxVisible)))
		sb.WriteString("\n")
	}
}

func (s *Model) renderDiscoverList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.discoverPlugins) == 0 {
			sb.WriteString(dimStyle.Render("No plugins available"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Add a marketplace in the Marketplaces tab"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("No matches"))
			sb.WriteString("\n")
		}
		return
	}

	// Calculate visible items (each takes 2-3 lines)
	maxItems := s.maxVisible / 2
	if maxItems < 3 {
		maxItems = 3
	}
	endIdx := min(s.scrollOffset+maxItems, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(DiscoverPluginItem)
		if !ok {
			continue
		}

		icon := "\u25cb"
		iconStyle := dimStyle
		if p.Installed {
			icon = "\u25cf"
			iconStyle = shared.SelectorStatusConnected
		}

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(p.Name)
		sb.WriteString(dimStyle.Render(" \u00b7 " + p.Marketplace))
		sb.WriteString("\n")

		// Line 2: description (indented, truncated to single line)
		if p.Description != "" {
			// 4 spaces indent + 3 for "...", cap at terminal width
			maxDescLen := s.width - 8
			if maxDescLen > 100 {
				maxDescLen = 100 // Cap at reasonable length
			}
			if maxDescLen > 20 {
				desc := shared.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.Render("    " + desc))
				sb.WriteString("\n")
			}
		}

		// Empty line for spacing between items
		sb.WriteString("\n")
	}

	// Scroll indicator
	if s.scrollOffset > 0 || endIdx < len(s.filteredItems) {
		remaining := len(s.filteredItems) - endIdx
		if remaining > 0 {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("  (%d more)", remaining)))
			sb.WriteString("\n")
		}
	}
}

func (s *Model) renderMarketplacesList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	addStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)

	// "+ Add Marketplace" as first item
	sb.WriteString(pluginCursor(s.selectedIdx == 0))
	sb.WriteString(addStyle.Render("+ Add Marketplace"))
	sb.WriteString("\n")

	if len(s.filteredItems) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("No marketplaces configured"))
		sb.WriteString("\n")
		return
	}

	sb.WriteString("\n")

	endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		m, ok := s.filteredItems[i].(MarketplaceItem)
		if !ok {
			continue
		}

		displayIdx := i + 1 // +1 for "Add Marketplace" item

		official := ""
		if m.IsOfficial {
			official = " \u273b"
		}

		sb.WriteString(pluginCursor(displayIdx == s.selectedIdx))
		sb.WriteString(shared.SelectorStatusConnected.Render("\u25cf"))
		sb.WriteString(" ")
		sb.WriteString(m.ID)
		sb.WriteString(dimStyle.Render(official))
		sb.WriteString("\n")

		// Details only for selected
		if displayIdx == s.selectedIdx {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("    %s", m.Source)))
			sb.WriteString("\n")
			stats := fmt.Sprintf("    %d available \u00b7 %d installed", m.Available, m.Installed)
			if m.LastUpdated != "" {
				stats += " \u00b7 " + m.LastUpdated
			}
			sb.WriteString(dimStyle.Render(stats))
			sb.WriteString("\n")
		}
	}
}

func (s *Model) renderInstalledDetail() string {
	if s.detailPlugin == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailPlugin
	maxValueLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	// Plugin name as title
	sb.WriteString(brightStyle.Render(p.FullName))
	sb.WriteString("\n\n")

	// Status
	icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)
	statusLabel := "Disabled"
	if p.Enabled {
		statusLabel = "Enabled"
	}
	sb.WriteString(dimStyle.Render("Status:  "))
	sb.WriteString(iconStyle.Render(icon + " " + statusLabel))
	sb.WriteString("\n")

	// Scope
	sb.WriteString(dimStyle.Render("Scope:   "))
	sb.WriteString(brightStyle.Render(string(p.Scope)))
	sb.WriteString("\n")

	// Version
	if p.Version != "" {
		sb.WriteString(dimStyle.Render("Version: "))
		sb.WriteString(brightStyle.Render(p.Version))
		sb.WriteString("\n")
	}

	// Author
	if p.Author != "" {
		sb.WriteString(dimStyle.Render("Author:  "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n")
	}

	// Description
	if p.Description != "" {
		sb.WriteString("\n")
		desc := shared.TruncateText(p.Description, maxValueLen)
		sb.WriteString(dimStyle.Render(desc))
		sb.WriteString("\n")
	}

	// Components
	components := buildComponentList(p)
	if len(components) > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("Components: " + strings.Join(components, ", ")))
		sb.WriteString("\n")
	}

	// Errors
	if len(p.Errors) > 0 {
		sb.WriteString("\n")
		sb.WriteString(shared.SelectorStatusError.Render("Errors:"))
		sb.WriteString("\n")
		for _, err := range p.Errors {
			sb.WriteString(shared.SelectorStatusError.Render("  \u2022 " + shared.TruncateText(err, maxValueLen)))
			sb.WriteString("\n")
		}
	}

	s.renderActions(&sb)
	s.renderFooter(&sb, "\u2191\u2193 navigate \u00b7 enter select \u00b7 esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderDiscoverDetail() string {
	if s.detailDiscover == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailDiscover
	maxValueLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)
	warnStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)

	// Plugin name as title
	sb.WriteString(brightStyle.Render(p.Name))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("from " + p.Marketplace))
	sb.WriteString("\n\n")

	// Description
	if p.Description != "" {
		desc := shared.TruncateText(p.Description, maxValueLen)
		sb.WriteString(dimStyle.Render(desc))
		sb.WriteString("\n\n")
	}

	// Author
	if p.Author != "" {
		sb.WriteString(dimStyle.Render("By: "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n\n")
	}

	// Warning
	sb.WriteString(warnStyle.Render("\u26a0 Make sure you trust a plugin before installing"))
	sb.WriteString("\n\n")

	s.renderActions(&sb)
	s.renderFooter(&sb, "enter select \u00b7 esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderMarketplaceDetail() string {
	if s.detailMarketplace == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	m := s.detailMarketplace

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	// Title
	sb.WriteString(brightStyle.Render(m.ID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(m.Source))
	sb.WriteString("\n\n")

	// Stats
	sb.WriteString(fmt.Sprintf("%d available plugins", m.Available))
	sb.WriteString("\n")

	if m.Installed > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Installed (%d):", m.Installed)))
		sb.WriteString("\n")
		// List installed plugins from this marketplace
		for _, p := range coreplugin.DefaultRegistry.List() {
			if idx := strings.Index(p.Source, "@"); idx != -1 {
				if p.Source[idx+1:] == m.ID {
					sb.WriteString("  \u25cf " + p.Name())
					sb.WriteString("\n")
				}
			}
		}
	}

	s.renderActions(&sb)
	s.renderFooter(&sb, "enter select \u00b7 esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderAddMarketplaceDialog() string {
	var sb strings.Builder
	maxInputLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	// Title
	sb.WriteString(brightStyle.Render("Add Marketplace"))
	sb.WriteString("\n\n")

	// Instructions
	sb.WriteString(dimStyle.Render("Enter marketplace source:"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Examples:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 https://github.com/owner/repo"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 owner/repo (GitHub shorthand)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 ./path/to/marketplace (local)"))
	sb.WriteString("\n\n")

	// Input field
	inputLine := s.addMarketplaceInput + "\u2502"
	if len(inputLine) > maxInputLen {
		inputLine = "\u2026" + inputLine[len(inputLine)-maxInputLen+1:]
	}
	sb.WriteString(brightStyle.Render("> " + inputLine))
	sb.WriteString("\n")

	s.renderFooter(&sb, "enter add \u00b7 esc cancel")
	return s.renderBox(sb.String())
}

func (s *Model) renderBrowsePlugins() string {
	var sb strings.Builder
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	// Title
	sb.WriteString(brightStyle.Render(s.browseMarketplaceID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("%d available plugins", len(s.browsePlugins))))
	sb.WriteString("\n\n")

	if len(s.browsePlugins) == 0 {
		sb.WriteString(dimStyle.Render("No plugins found"))
		sb.WriteString("\n")
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.browsePlugins))

		for i := s.scrollOffset; i < endIdx; i++ {
			p := s.browsePlugins[i]

			icon := "\u25cb"
			iconStyle := dimStyle
			if p.Installed {
				icon = "\u25cf"
				iconStyle = shared.SelectorStatusConnected
			}

			sb.WriteString(pluginCursor(i == s.selectedIdx))
			sb.WriteString(iconStyle.Render(icon))
			sb.WriteString(" ")
			sb.WriteString(p.Name)
			sb.WriteString("\n")

			if p.Description != "" && i == s.selectedIdx {
				desc := shared.TruncateText(p.Description, s.width-10)
				sb.WriteString(dimStyle.Render("    " + desc))
				sb.WriteString("\n")
			}
		}
	}

	s.renderFooter(&sb, "\u2191\u2193 navigate \u00b7 enter details \u00b7 esc back")
	return s.renderBox(sb.String())
}

// Helper functions

// renderActions renders the action list for detail views
func (s *Model) renderActions(sb *strings.Builder) {
	sb.WriteString("\n")
	for i, action := range s.actions {
		cursor := "  "
		if i == s.actionIdx {
			cursor = "> "
		}
		sb.WriteString(cursor + action.Label + "\n")
	}
}

func (s *Model) renderFooter(sb *strings.Builder, hint string) {
	sb.WriteString("\n")
	if s.isLoading {
		spinnerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent)
		sb.WriteString(spinnerStyle.Render("  \u25d0 " + s.loadingMsg))
		sb.WriteString("\n\n")
	} else if s.lastMessage != "" {
		if s.isError {
			sb.WriteString(shared.SelectorStatusError.Render("  \u26a0 " + s.lastMessage))
		} else {
			successStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)
			sb.WriteString(successStyle.Render("  \u2713 " + s.lastMessage))
		}
		sb.WriteString("\n\n")
	}
	sb.WriteString(s.renderHints(hint))
}

// setError sets an error message
func (s *Model) setError(msg string) {
	s.lastMessage = msg
	s.isError = true
}

// setSuccess sets a success message
func (s *Model) setSuccess(msg string) {
	s.lastMessage = msg
	s.isError = false
}

// clearMessage clears the status message
func (s *Model) clearMessage() {
	s.lastMessage = ""
	s.isError = false
}

// renderHints renders keyboard hints in a clean format
func (s *Model) renderHints(hint string) string {
	hintStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
	return hintStyle.Render(hint)
}

func (s *Model) renderBox(content string) string {
	// Full screen layout like Claude Code
	style := lipgloss.NewStyle().
		Padding(1, 2).
		Width(s.width - 4)
	return style.Render(content)
}

func pluginStatusIconAndStyle(enabled bool) (string, lipgloss.Style) {
	if enabled {
		return "\u25cf", shared.SelectorStatusConnected
	}
	return "\u25cb", shared.SelectorStatusNone
}

func pluginCursor(selected bool) string {
	if selected {
		return "\u276f "
	}
	return "  "
}

// buildComponentList builds a list of component counts for display
func buildComponentList(p *PluginItem) []string {
	type componentCount struct {
		name  string
		count int
	}
	counts := []componentCount{
		{"Skills", p.Skills},
		{"Agents", p.Agents},
		{"Commands", p.Commands},
		{"Hooks", p.Hooks},
		{"MCP", p.MCP},
		{"LSP", p.LSP},
	}

	var result []string
	for _, c := range counts {
		if c.count > 0 {
			result = append(result, fmt.Sprintf("%s: %d", c.name, c.count))
		}
	}
	return result
}

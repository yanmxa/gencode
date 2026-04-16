// Package plugin provides the plugin selector feature.
package pluginui

import (
	"os"

	coreplugin "github.com/yanmxa/gencode/internal/extension/plugin"
)

// Tab represents the active tab in the plugin selector
type Tab int

const (
	tabDiscover Tab = iota
	tabInstalled
	tabMarketplaces
)

// Level represents the navigation level within the plugin selector
type Level int

const (
	levelTabList Level = iota
	levelDetail
	levelInstallOptions
	levelAddMarketplace
	levelBrowsePlugins
)

// Action represents an action available in detail view
type Action struct {
	Label  string
	Action string
}

// pluginItem represents a plugin in the selector
type pluginItem struct {
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
	Marketplace string
}

// discoverPluginItem represents a plugin available in a marketplace
type discoverPluginItem struct {
	Name        string
	Description string
	Marketplace string
	Author      string
	Installed   bool
	Homepage    string
	Version     string
}

// marketplaceItem represents a marketplace in the selector
type marketplaceItem struct {
	ID          string
	Source      string
	SourceType  string
	Available   int
	Installed   int
	LastUpdated string
	IsOfficial  bool
}

// Model holds state for the plugin selector
type Model struct {
	active      bool
	width       int
	height      int
	lastMessage string
	isError     bool
	maxVisible  int
	isLoading   bool
	loadingMsg  string

	activeTab Tab

	installedPlugins  map[coreplugin.Scope][]pluginItem
	installedScopes   []coreplugin.Scope
	installedFlatList []pluginItem
	discoverPlugins   []discoverPluginItem
	marketplaces      []marketplaceItem

	level        Level
	selectedIdx   int
	scrollOffset  int
	detailScroll  int

	searchQuery   string
	filteredItems []any

	detailPlugin      *pluginItem
	detailDiscover    *discoverPluginItem
	detailMarketplace *marketplaceItem
	actions           []Action
	actionIdx         int
	parentIdx         int

	addMarketplaceInput string
	addDialogCursor     int

	browseMarketplaceID string
	browsePlugins       []discoverPluginItem

	marketplaceManager *coreplugin.MarketplaceManager
	installer          *coreplugin.Installer
}

// Plugin messages
type EnableMsg struct{ PluginName string }
type DisableMsg struct{ PluginName string }

type InstallMsg struct {
	PluginName  string
	Marketplace string
	Scope       coreplugin.Scope
}

type UninstallMsg struct{ PluginName string }

type InstallResultMsg struct {
	PluginName string
	Success    bool
	Error      error
}

type MarketplaceRemoveMsg struct{ ID string }

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
		maxVisible:         15,
		activeTab:          tabInstalled,
		installedPlugins:   make(map[coreplugin.Scope][]pluginItem),
		marketplaceManager: coreplugin.NewMarketplaceManager(cwd),
		installer:          coreplugin.NewInstaller(coreplugin.DefaultRegistry, cwd),
	}
}

// IsActive returns whether the selector is active
func (s *Model) IsActive() bool {
	return s.active
}

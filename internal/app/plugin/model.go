// Package plugin provides the plugin selector feature.
package plugin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

// Tab represents the active tab in the plugin selector
type Tab int

const (
	TabDiscover Tab = iota
	TabInstalled
	TabMarketplaces
)

// Level represents the navigation level within the plugin selector
type Level int

const (
	LevelTabList Level = iota
	LevelDetail
	LevelInstallOptions
	LevelAddMarketplace
	LevelBrowsePlugins
)

// Action represents an action available in detail view
type Action struct {
	Label  string
	Action string
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
	Marketplace string
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

	installedPlugins  map[coreplugin.Scope][]PluginItem
	installedScopes   []coreplugin.Scope
	installedFlatList []PluginItem
	discoverPlugins   []DiscoverPluginItem
	marketplaces      []MarketplaceItem

	level        Level
	selectedIdx  int
	scrollOffset int

	searchQuery   string
	filteredItems []any

	detailPlugin      *PluginItem
	detailDiscover    *DiscoverPluginItem
	detailMarketplace *MarketplaceItem
	actions           []Action
	actionIdx         int
	parentIdx         int

	installScopes   []coreplugin.Scope
	installScopeIdx int

	addMarketplaceInput string
	addDialogCursor     int

	browseMarketplaceID string
	browsePlugins       []DiscoverPluginItem

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

type MarketplaceAddMsg struct{ Source string }
type MarketplaceRemoveMsg struct{ ID string }
type MarketplaceSyncMsg struct{ ID string }

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
		activeTab:          TabInstalled,
		installedPlugins:   make(map[coreplugin.Scope][]PluginItem),
		marketplaceManager: coreplugin.NewMarketplaceManager(cwd),
		installer:          coreplugin.NewInstaller(coreplugin.DefaultRegistry, cwd),
	}
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
func (s *Model) NextTab() { s.switchTab((s.activeTab + 1) % 3) }
func (s *Model) PrevTab() { s.switchTab((s.activeTab + 2) % 3) }

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
	s.clearMessage()
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
	s.clearMessage()
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
			maxIdx++
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
		if s.selectedIdx == 0 {
			s.level = LevelAddMarketplace
			s.addMarketplaceInput = ""
			s.addDialogCursor = 0
			return
		}
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

// toggleSelectedPlugin toggles enable/disable for the selected plugin
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

// HandleKeypress handles a keypress and returns a command if needed
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	if s.level == LevelAddMarketplace {
		return s.handleAddMarketplaceKeypress(key)
	}
	if s.level == LevelDetail || s.level == LevelInstallOptions {
		return s.handleDetailKeypress(key)
	}
	if s.level == LevelBrowsePlugins {
		return s.handleBrowseKeypress(key)
	}
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
		if s.addMarketplaceInput == "" {
			input = strings.TrimPrefix(input, "[")
		}
		input = strings.TrimSuffix(input, "]")
		if input != "" {
			s.addMarketplaceInput += input
		}
		return nil
	}
	return nil
}

func (s *Model) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
	if s.handleNavigationKey(key, true) {
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
	if s.handleNavigationKey(key, true) {
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

// handleNavigationKey handles common up/down navigation keys, returns true if handled.
func (s *Model) handleNavigationKey(key tea.KeyMsg, vimKeys bool) bool {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return true
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return true
	case tea.KeyRunes:
		if vimKeys {
			switch key.String() {
			case "k":
				s.MoveUp()
				return true
			case "j":
				s.MoveDown()
				return true
			}
		}
	}
	return false
}

func (s *Model) handleListKeypress(key tea.KeyMsg) tea.Cmd {
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

	if s.handleNavigationKey(key, s.searchQuery == "") {
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

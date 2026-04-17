// Plugin selector: model, state, runtime, keymap, navigation, actions, commands, load, reset.
package user

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

	"github.com/yanmxa/gencode/internal/app/kit"
	coreplugin "github.com/yanmxa/gencode/internal/plugin"
)

// ── Types ────────────────────────────────────────────────────────────────────

// pluginTab represents the active tab in the plugin selector
type pluginTab int

const (
	pluginTabDiscover pluginTab = iota
	pluginTabInstalled
	pluginTabMarketplaces
)

// pluginLevel represents the navigation level within the plugin selector
type pluginLevel int

const (
	pluginLevelTabList pluginLevel = iota
	pluginLevelDetail
	pluginLevelInstallOptions
	pluginLevelAddMarketplace
	pluginLevelBrowsePlugins
)

type pluginAction struct {
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

// pluginDiscoverItem represents a plugin available in a marketplace
type pluginDiscoverItem struct {
	Name        string
	Description string
	Marketplace string
	Author      string
	Installed   bool
	Homepage    string
	Version     string
}

// pluginMarketplaceItem represents a marketplace in the selector
type pluginMarketplaceItem struct {
	ID          string
	Source      string
	SourceType  string
	Available   int
	Installed   int
	LastUpdated string
	IsOfficial  bool
}

// PluginSelector holds state for the plugin selector
type PluginSelector struct {
	registry *coreplugin.Registry

	active      bool
	width       int
	height      int
	lastMessage string
	isError     bool
	maxVisible  int
	isLoading   bool
	loadingMsg  string

	activeTab pluginTab

	installedPlugins  map[coreplugin.Scope][]pluginItem
	installedScopes   []coreplugin.Scope
	installedFlatList []pluginItem
	discoverPlugins   []pluginDiscoverItem
	marketplaces      []pluginMarketplaceItem

	level        pluginLevel
	selectedIdx  int
	scrollOffset int
	detailScroll int

	searchQuery   string
	filteredItems []any

	detailPlugin      *pluginItem
	detailDiscover    *pluginDiscoverItem
	detailMarketplace *pluginMarketplaceItem
	actions           []pluginAction
	actionIdx         int
	parentIdx         int

	addMarketplaceInput string
	addDialogCursor     int

	browseMarketplaceID string
	browsePlugins       []pluginDiscoverItem

	marketplaceManager *coreplugin.MarketplaceManager
	installer          *coreplugin.Installer
}

// Plugin messages
type PluginEnableMsg struct{ PluginName string }
type PluginDisableMsg struct{ PluginName string }

type PluginInstallMsg struct {
	PluginName  string
	Marketplace string
	Scope       coreplugin.Scope
}

type PluginUninstallMsg struct{ PluginName string }

type PluginInstallResultMsg struct {
	PluginName string
	Success    bool
	Error      error
}

type PluginMarketplaceRemoveMsg struct{ ID string }

type PluginMarketplaceSyncResultMsg struct {
	ID      string
	Success bool
	Error   error
}

// NewPluginSelector creates a new PluginSelector
func NewPluginSelector(reg *coreplugin.Registry) PluginSelector {
	cwd, _ := os.Getwd()
	return PluginSelector{
		registry:           reg,
		active:             false,
		maxVisible:         15,
		activeTab:          pluginTabInstalled,
		installedPlugins:   make(map[coreplugin.Scope][]pluginItem),
		marketplaceManager: coreplugin.NewMarketplaceManager(cwd),
		installer:          coreplugin.NewInstaller(reg, cwd),
	}
}

// IsActive returns whether the selector is active
func (s *PluginSelector) IsActive() bool {
	return s.active
}

// ── Runtime ──────────────────────────────────────────────────────────────────

// PluginRuntime defines the callbacks the plugin selector needs from the parent app model.
type PluginRuntime interface {
	GetCwd() string
	ReloadPluginBackedState() error
}

// UpdatePlugin routes plugin management messages.
func UpdatePlugin(rt PluginRuntime, state *PluginSelector, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case PluginEnableMsg:
		state.HandleEnable(msg.PluginName)
		return nil, true

	case PluginDisableMsg:
		state.HandleDisable(msg.PluginName)
		return nil, true

	case PluginUninstallMsg:
		state.HandleUninstall(msg.PluginName)
		return nil, true

	case PluginInstallMsg:
		return pluginInstallCmd(state.registry, rt.GetCwd(), msg), true

	case PluginInstallResultMsg:
		state.HandleInstallResult(msg)
		if msg.Success {
			_ = rt.ReloadPluginBackedState()
		}
		return nil, true

	case PluginMarketplaceRemoveMsg:
		state.HandleMarketplaceRemove(msg.ID)
		return nil, true

	case PluginMarketplaceSyncResultMsg:
		state.HandleMarketplaceSync(msg)
		return nil, true
	}
	return nil, false
}

// pluginInstallCmd creates a tea.Cmd that installs the requested plugin.
func pluginInstallCmd(reg *coreplugin.Registry, cwd string, msg PluginInstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := coreplugin.NewInstaller(reg, cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return PluginInstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

// ── Keymap ───────────────────────────────────────────────────────────────────

// HandleKeypress handles a keypress and returns a command if needed.
func (s *PluginSelector) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	if s.level == pluginLevelAddMarketplace {
		return s.handleAddMarketplaceKeypress(key)
	}
	if s.level == pluginLevelDetail || s.level == pluginLevelInstallOptions {
		return s.handleDetailKeypress(key)
	}
	if s.level == pluginLevelBrowsePlugins {
		return s.handleBrowseKeypress(key)
	}
	return s.handleListKeypress(key)
}

func (s *PluginSelector) handleAddMarketplaceKeypress(key tea.KeyMsg) tea.Cmd {
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

func (s *PluginSelector) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
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

func (s *PluginSelector) handleBrowseKeypress(key tea.KeyMsg) tea.Cmd {
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
			s.level = pluginLevelDetail
		}
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
	}
	return nil
}

// handleNavigationKey handles common up/down navigation keys, returns true if handled.
func (s *PluginSelector) handleNavigationKey(key tea.KeyMsg, vimKeys bool) bool {
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

func (s *PluginSelector) handleListKeypress(key tea.KeyMsg) tea.Cmd {
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
		return func() tea.Msg { return kit.DismissedMsg{} }
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

// handleListRuneKey handles rune key input in list view.
func (s *PluginSelector) handleListRuneKey(r string) tea.Cmd {
	if s.searchQuery == "" {
		switch r {
		case "l":
			s.enterDetail()
			return nil
		case " ":
			return s.toggleSelectedPlugin()
		case "u":
			return s.handleMarketplaceAction(func(m pluginMarketplaceItem) tea.Cmd {
				return s.syncMarketplace(m.ID)
			})
		case "r":
			return s.handleMarketplaceAction(func(m pluginMarketplaceItem) tea.Cmd {
				return func() tea.Msg { return PluginMarketplaceRemoveMsg{ID: m.ID} }
			})
		}
	}
	s.searchQuery += r
	s.updateFilter()
	return nil
}

// handleMarketplaceAction executes an action on the selected marketplace.
func (s *PluginSelector) handleMarketplaceAction(action func(pluginMarketplaceItem) tea.Cmd) tea.Cmd {
	if s.activeTab != pluginTabMarketplaces || s.selectedIdx == 0 {
		return nil
	}
	mktIdx := s.selectedIdx - 1
	if mktIdx < len(s.filteredItems) {
		if m, ok := s.filteredItems[mktIdx].(pluginMarketplaceItem); ok {
			return action(m)
		}
	}
	return nil
}

// ── Navigation ───────────────────────────────────────────────────────────────

// Tab navigation
func (s *PluginSelector) NextTab() { s.switchTab((s.activeTab + 1) % 3) }
func (s *PluginSelector) PrevTab() { s.switchTab((s.activeTab + 2) % 3) }

func (s *PluginSelector) switchTab(tab pluginTab) {
	s.activeTab = tab
	s.resetListState()
	s.resetDetailState()
	s.resetBrowseState()
	s.searchQuery = ""
	s.refreshCurrentTab()
}

// updateFilter filters items based on search query
func (s *PluginSelector) updateFilter() {
	query := strings.ToLower(s.searchQuery)
	s.filteredItems = s.filterItemsForTab(query)
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// filterItemsForTab returns filtered items based on the active tab and query
func (s *PluginSelector) filterItemsForTab(query string) []any {
	switch s.activeTab {
	case pluginTabInstalled:
		return pluginFilterItems(s.installedFlatList, query, func(p pluginItem) []string {
			return []string{p.Name, p.Description}
		})
	case pluginTabDiscover:
		return pluginFilterItems(s.discoverPlugins, query, func(p pluginDiscoverItem) []string {
			return []string{p.Name, p.Description, p.Marketplace}
		})
	case pluginTabMarketplaces:
		return pluginFilterItems(s.marketplaces, query, func(m pluginMarketplaceItem) []string {
			return []string{m.ID, m.Source}
		})
	default:
		return nil
	}
}

// pluginFilterItems is a generic filter function for any slice type
func pluginFilterItems[T any](items []T, query string, getFields func(T) []string) []any {
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
			if kit.FuzzyMatch(strings.ToLower(field), query) {
				result = append(result, item)
				break
			}
		}
	}
	return result
}

// Navigation
func (s *PluginSelector) MoveUp() {
	s.clearMessage()
	switch s.level {
	case pluginLevelDetail, pluginLevelInstallOptions:
		if s.actionIdx > 0 {
			s.actionIdx--
		} else if s.detailScroll > 0 {
			s.detailScroll--
		}
	default:
		if s.selectedIdx > 0 {
			s.selectedIdx--
			s.ensureVisible()
		}
	}
}

func (s *PluginSelector) MoveDown() {
	s.clearMessage()
	switch s.level {
	case pluginLevelDetail, pluginLevelInstallOptions:
		if s.actionIdx < len(s.actions)-1 {
			s.actionIdx++
		} else {
			s.detailScroll++
		}
	default:
		maxIdx := s.getMaxIndex()
		if s.selectedIdx < maxIdx {
			s.selectedIdx++
			s.ensureVisible()
		}
	}
}

// getMaxIndex returns the maximum selectable index for the current view.
func (s *PluginSelector) getMaxIndex() int {
	switch s.level {
	case pluginLevelBrowsePlugins:
		return len(s.browsePlugins) - 1
	default:
		maxIdx := len(s.filteredItems) - 1
		if s.activeTab == pluginTabMarketplaces {
			maxIdx++
		}
		return maxIdx
	}
}

func (s *PluginSelector) ensureVisible() {
	visible := s.maxVisible
	switch s.level {
	case pluginLevelBrowsePlugins:
		visible = max(4, s.height-14)
	default:
		switch s.activeTab {
		case pluginTabDiscover:
			visible = max(3, (s.height-14)/3)
		case pluginTabMarketplaces:
			visible = max(4, (s.height-14)/2)
		default:
			visible = max(4, s.height-14)
		}
	}
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+visible {
		s.scrollOffset = s.selectedIdx - visible + 1
	}
}

// enterDetail enters the detail view for the selected item.
func (s *PluginSelector) enterDetail() {
	s.parentIdx = s.selectedIdx

	switch s.activeTab {
	case pluginTabInstalled:
		s.enterInstalledDetail()
	case pluginTabDiscover:
		s.enterDiscoverDetail()
	case pluginTabMarketplaces:
		s.enterMarketplaceDetail()
	}
}

func (s *PluginSelector) enterInstalledDetail() {
	if s.selectedIdx >= len(s.filteredItems) {
		return
	}
	if p, ok := s.filteredItems[s.selectedIdx].(pluginItem); ok {
		s.detailPlugin = &p
		s.actions = s.buildInstalledActions(p)
		s.actionIdx = 0
		s.level = pluginLevelDetail
	}
}

func (s *PluginSelector) enterDiscoverDetail() {
	if s.selectedIdx >= len(s.filteredItems) {
		return
	}
	if p, ok := s.filteredItems[s.selectedIdx].(pluginDiscoverItem); ok {
		s.detailDiscover = &p
		s.actions = s.buildDiscoverActions(p)
		s.actionIdx = 0
		s.level = pluginLevelDetail
	}
}

func (s *PluginSelector) enterMarketplaceDetail() {
	if s.selectedIdx == 0 {
		s.level = pluginLevelAddMarketplace
		s.addMarketplaceInput = ""
		s.addDialogCursor = 0
		return
	}
	mktIdx := s.selectedIdx - 1
	if mktIdx >= len(s.filteredItems) {
		return
	}
	if m, ok := s.filteredItems[mktIdx].(pluginMarketplaceItem); ok {
		s.detailMarketplace = &m
		s.actions = s.buildMarketplaceActions(m)
		s.actionIdx = 0
		s.level = pluginLevelDetail
	}
}

// goBack returns to the previous view.
func (s *PluginSelector) goBack() bool {
	switch s.level {
	case pluginLevelDetail:
		s.level = pluginLevelTabList
		s.selectedIdx = s.parentIdx
		s.resetDetailState()
		s.clearMessage()
		return true
	case pluginLevelInstallOptions:
		s.level = pluginLevelDetail
		s.actions = s.buildDiscoverActions(*s.detailDiscover)
		s.actionIdx = 0
		return true
	case pluginLevelAddMarketplace:
		s.level = pluginLevelTabList
		s.addMarketplaceInput = ""
		return true
	case pluginLevelBrowsePlugins:
		s.level = pluginLevelDetail
		s.resetBrowseState()
		s.selectedIdx = 0
		return true
	}
	return false
}

// ── Actions ──────────────────────────────────────────────────────────────────

// buildInstalledActions returns actions for an installed plugin.
func (s *PluginSelector) buildInstalledActions(p pluginItem) []pluginAction {
	actions := []pluginAction{}
	if p.Enabled {
		actions = append(actions, pluginAction{Label: "Disable plugin", Action: "disable"})
	} else {
		actions = append(actions, pluginAction{Label: "Enable plugin", Action: "enable"})
	}
	actions = append(actions,
		pluginAction{Label: "Uninstall", Action: "uninstall"},
		pluginAction{Label: "Back to plugin list", Action: "back"},
	)
	return actions
}

// buildDiscoverActions returns actions for a discoverable plugin.
func (s *PluginSelector) buildDiscoverActions(p pluginDiscoverItem) []pluginAction {
	actions := []pluginAction{}
	if !p.Installed {
		actions = append(actions,
			pluginAction{Label: "Install for you (user scope)", Action: "install_user"},
			pluginAction{Label: "Install for all collaborators (project scope)", Action: "install_project"},
			pluginAction{Label: "Install for you, in this repo only (local scope)", Action: "install_local"},
		)
	} else {
		actions = append(actions, pluginAction{Label: "Already installed", Action: "none"})
	}
	if p.Homepage != "" {
		actions = append(actions, pluginAction{Label: "Open homepage", Action: "homepage"})
	}
	actions = append(actions, pluginAction{Label: "Back to plugin list", Action: "back"})
	return actions
}

// buildMarketplaceActions returns actions for a marketplace.
func (s *PluginSelector) buildMarketplaceActions(m pluginMarketplaceItem) []pluginAction {
	return []pluginAction{
		{Label: fmt.Sprintf("Browse plugins (%d)", m.Available), Action: "browse"},
		{Label: "Update marketplace", Action: "update"},
		{Label: "Remove marketplace", Action: "remove"},
		{Label: "Back", Action: "back"},
	}
}

// executeAction executes the currently selected action.
func (s *PluginSelector) executeAction() tea.Cmd {
	if s.actionIdx >= len(s.actions) {
		return nil
	}
	action := s.actions[s.actionIdx]

	switch action.Action {
	case "enable":
		if s.detailPlugin != nil {
			return func() tea.Msg { return PluginEnableMsg{PluginName: s.detailPlugin.FullName} }
		}
	case "disable":
		if s.detailPlugin != nil {
			return func() tea.Msg { return PluginDisableMsg{PluginName: s.detailPlugin.FullName} }
		}
	case "uninstall":
		if s.detailPlugin != nil {
			return func() tea.Msg { return PluginUninstallMsg{PluginName: s.detailPlugin.FullName} }
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
			return func() tea.Msg { return PluginMarketplaceRemoveMsg{ID: s.detailMarketplace.ID} }
		}
	case "back":
		s.goBack()
	}
	return nil
}

// installPlugin creates an install command for the selected plugin.
func (s *PluginSelector) installPlugin(scope coreplugin.Scope) tea.Cmd {
	if s.detailDiscover == nil {
		return nil
	}
	name := s.detailDiscover.Name
	marketplace := s.detailDiscover.Marketplace
	s.isLoading = true
	s.loadingMsg = fmt.Sprintf("Installing %s...", name)
	return func() tea.Msg {
		return PluginInstallMsg{
			PluginName:  name,
			Marketplace: marketplace,
			Scope:       scope,
		}
	}
}

// browseMarketplace enters the browse view for a marketplace.
func (s *PluginSelector) browseMarketplace() {
	if s.detailMarketplace == nil {
		return
	}

	s.browseMarketplaceID = s.detailMarketplace.ID
	s.browsePlugins = []pluginDiscoverItem{}

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

	s.level = pluginLevelBrowsePlugins
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// syncMarketplace creates a sync command for a marketplace.
func (s *PluginSelector) syncMarketplace(id string) tea.Cmd {
	s.isLoading = true
	s.loadingMsg = fmt.Sprintf("Syncing %s...", id)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := s.marketplaceManager.Sync(ctx, id); err != nil {
			return PluginMarketplaceSyncResultMsg{ID: id, Success: false, Error: err}
		}
		return PluginMarketplaceSyncResultMsg{ID: id, Success: true}
	}
}

// toggleSelectedPlugin toggles enable/disable for the selected plugin.
func (s *PluginSelector) toggleSelectedPlugin() tea.Cmd {
	if s.activeTab == pluginTabInstalled && s.level == pluginLevelTabList && s.selectedIdx < len(s.filteredItems) {
		if p, ok := s.filteredItems[s.selectedIdx].(pluginItem); ok {
			if p.Enabled {
				return func() tea.Msg { return PluginDisableMsg{PluginName: p.FullName} }
			}
			return func() tea.Msg { return PluginEnableMsg{PluginName: p.FullName} }
		}
	}
	return nil
}

// HandleEnable handles enabling a plugin.
func (s *PluginSelector) HandleEnable(name string) {
	if err := s.registry.Enable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to enable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Enabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleDisable handles disabling a plugin.
func (s *PluginSelector) HandleDisable(name string) {
	if err := s.registry.Disable(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to disable: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Disabled %s", name))
	}
	s.refreshAndUpdateView()
}

// HandleUninstall handles uninstalling a plugin.
func (s *PluginSelector) HandleUninstall(name string) {
	if err := s.installer.Uninstall(name, coreplugin.ScopeUser); err != nil {
		s.setError(fmt.Sprintf("Failed to uninstall: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Uninstalled %s", name))
		s.goBack()
	}
	s.refreshAndUpdateView()
}

// HandleInstallResult handles the result of plugin installation.
func (s *PluginSelector) HandleInstallResult(msg PluginInstallResultMsg) {
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
func (s *PluginSelector) HandleMarketplaceSync(msg PluginMarketplaceSyncResultMsg) {
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
func (s *PluginSelector) HandleMarketplaceRemove(id string) {
	if err := s.marketplaceManager.Remove(id); err != nil {
		s.setError(fmt.Sprintf("Failed to remove: %v", err))
	} else {
		s.setSuccess(fmt.Sprintf("Removed %s", id))
		s.goBack()
	}
	s.refreshMarketplaces()
}

// setError sets an error message.
func (s *PluginSelector) setError(msg string) {
	s.lastMessage = msg
	s.isError = true
}

// setSuccess sets a success message.
func (s *PluginSelector) setSuccess(msg string) {
	s.lastMessage = msg
	s.isError = false
}

// clearMessage clears the status message.
func (s *PluginSelector) clearMessage() {
	s.lastMessage = ""
	s.isError = false
}

// ── Commands ─────────────────────────────────────────────────────────────────

// HandlePluginCommand dispatches /plugin subcommands.
func HandlePluginCommand(ctx context.Context, selector *PluginSelector, cwd string, width, height int, args string) (string, error) {
	if selector.registry.Count() == 0 {
		if err := selector.registry.Load(ctx, cwd); err != nil {
			return fmt.Sprintf("Failed to load plugins: %v", err), nil
		}
		_ = selector.registry.LoadClaudePlugins(ctx)
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		if err := selector.EnterSelect(width, height); err != nil {
			return fmt.Sprintf("Failed to open plugin selector: %v", err), nil
		}
		return "", nil
	}

	subCmd := strings.ToLower(parts[0])
	var pluginName string
	if len(parts) > 1 {
		pluginName = parts[1]
	}

	switch subCmd {
	case "list":
		return pluginHandleList(selector.registry)
	case "install":
		return pluginHandleInstall(selector.registry, ctx, cwd, parts[1:])
	case "marketplace":
		return pluginHandleMarketplace(ctx, cwd, parts[1:])
	case "enable":
		return pluginHandleEnable(selector.registry, ctx, pluginName)
	case "disable":
		return pluginHandleDisable(selector.registry, ctx, pluginName)
	case "info":
		return pluginHandleInfo(selector.registry, pluginName)
	case "errors":
		return pluginHandleErrors(selector.registry)
	default:
		return pluginHandleInfo(selector.registry, subCmd)
	}
}

// pluginHandleList shows all installed plugins.
func pluginHandleList(reg *coreplugin.Registry) (string, error) {
	plugins := reg.List()

	if len(plugins) == 0 {
		return "No plugins installed.\n\nInstall with: gen plugin install <plugin>@<marketplace>", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugins (%d installed, %d enabled):\n\n",
		reg.Count(),
		reg.EnabledCount())

	for _, p := range plugins {
		pluginWriteSummary(&sb, p)
	}

	sb.WriteString("\nLegend: ● enabled  ○ disabled  👤 user  📁 project  💻 local")
	sb.WriteString("\n\nCommands:\n")
	sb.WriteString("  /plugin install <ref>    Install a plugin from a marketplace\n")
	sb.WriteString("  /plugin marketplace ...  Manage plugin marketplaces\n")
	sb.WriteString("  /plugin enable <name>   Enable a plugin\n")
	sb.WriteString("  /plugin disable <name>  Disable a plugin\n")
	sb.WriteString("  /plugin info <name>     Show plugin details\n")

	return sb.String(), nil
}

func pluginWriteSummary(sb *strings.Builder, p *coreplugin.Plugin) {
	status := "○"
	if p.Enabled {
		status = "●"
	}

	fmt.Fprintf(sb, "  %s %s %s (%s)\n", status, p.Scope.Icon(), p.FullName(), p.Scope)

	if p.Manifest.Description != "" {
		fmt.Fprintf(sb, "      %s\n", p.Manifest.Description)
	}

	components := pluginFormatComponentCounts(p)
	if len(components) > 0 {
		fmt.Fprintf(sb, "      [%s]\n", strings.Join(components, ", "))
	}
}

func pluginFormatComponentCounts(p *coreplugin.Plugin) []string {
	var components []string
	if n := len(p.Components.Skills); n > 0 {
		components = append(components, fmt.Sprintf("%d skills", n))
	}
	if n := len(p.Components.Agents); n > 0 {
		components = append(components, fmt.Sprintf("%d agents", n))
	}
	if n := len(p.Components.Commands); n > 0 {
		components = append(components, fmt.Sprintf("%d commands", n))
	}
	if p.Components.Hooks != nil {
		if n := len(p.Components.Hooks.Hooks); n > 0 {
			components = append(components, fmt.Sprintf("%d hooks", n))
		}
	}
	if n := len(p.Components.MCP); n > 0 {
		components = append(components, fmt.Sprintf("%d MCP", n))
	}
	if n := len(p.Components.LSP); n > 0 {
		components = append(components, fmt.Sprintf("%d LSP", n))
	}
	return components
}

// pluginHandleEnable enables a plugin.
func pluginHandleEnable(reg *coreplugin.Registry, _ context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin enable <plugin-name>", nil
	}

	if err := reg.Enable(name, coreplugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to enable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Enabled plugin '%s'\n\nRun /reload-plugins to apply changes in the current session.", name), nil
}

// pluginHandleDisable disables a plugin.
func pluginHandleDisable(reg *coreplugin.Registry, _ context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin disable <plugin-name>", nil
	}

	if err := reg.Disable(name, coreplugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to disable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Disabled plugin '%s'\n\nRun /reload-plugins to apply changes in the current session.", name), nil
}

// pluginHandleInfo shows detailed info for a plugin.
func pluginHandleInfo(reg *coreplugin.Registry, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin info <plugin-name>", nil
	}

	p, ok := reg.Get(name)
	if !ok {
		return fmt.Sprintf("Plugin not found: %s\n\nUse /plugin list to see available plugins.", name), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugin: %s\n", p.FullName())
	fmt.Fprintf(&sb, "Scope: %s\n", p.Scope)
	fmt.Fprintf(&sb, "Enabled: %v\n", p.Enabled)
	fmt.Fprintf(&sb, "Path: %s\n", p.Path)

	pluginWriteOptionalField(&sb, "Version", p.Manifest.Version)
	pluginWriteOptionalField(&sb, "Description", p.Manifest.Description)
	if p.Manifest.Author != nil {
		pluginWriteOptionalField(&sb, "Author", p.Manifest.Author.Name)
	}
	pluginWriteOptionalField(&sb, "Repository", p.Manifest.Repository)

	sb.WriteString("\nComponents:\n")
	pluginWriteComponentCount(&sb, "Commands", len(p.Components.Commands))
	pluginWriteComponentCount(&sb, "Skills", len(p.Components.Skills))
	pluginWriteComponentCount(&sb, "Agents", len(p.Components.Agents))
	if p.Components.Hooks != nil {
		pluginWriteComponentCount(&sb, "Hook events", len(p.Components.Hooks.Hooks))
	}
	pluginWriteComponentCount(&sb, "MCP servers", len(p.Components.MCP))
	pluginWriteComponentCount(&sb, "LSP servers", len(p.Components.LSP))

	if len(p.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range p.Errors {
			fmt.Fprintf(&sb, "  - %s\n", err)
		}
	}

	return sb.String(), nil
}

func pluginWriteOptionalField(sb *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(sb, "%s: %s\n", label, value)
	}
}

func pluginWriteComponentCount(sb *strings.Builder, label string, count int) {
	if count > 0 {
		fmt.Fprintf(sb, "  %s: %d\n", label, count)
	}
}

// pluginHandleErrors shows all plugin errors.
func pluginHandleErrors(reg *coreplugin.Registry) (string, error) {
	plugins := reg.List()

	var sb strings.Builder
	hasErrors := false

	for _, p := range plugins {
		if len(p.Errors) > 0 {
			hasErrors = true
			fmt.Fprintf(&sb, "%s:\n", p.FullName())
			for _, err := range p.Errors {
				fmt.Fprintf(&sb, "  - %s\n", err)
			}
			sb.WriteString("\n")
		}
	}

	if !hasErrors {
		return "No plugin errors.", nil
	}

	return sb.String(), nil
}

// pluginHandleInstall installs a plugin from a configured marketplace.
func pluginHandleInstall(reg *coreplugin.Registry, ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /plugin install <plugin>@<marketplace> [user|project|local]", nil
	}
	if len(args) > 2 {
		return "Usage: /plugin install <plugin>@<marketplace> [user|project|local]", nil
	}

	scope, err := pluginParseScopeArg("")
	if err != nil {
		return "", err
	}
	if len(args) == 2 {
		scope, err = pluginParseScopeArg(args[1])
		if err != nil {
			return err.Error(), nil
		}
	}

	installer := coreplugin.NewInstaller(reg, cwd)
	if err := installer.LoadMarketplaces(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	ref := args[0]
	if err := installer.Install(ctx, ref, scope); err != nil {
		return fmt.Sprintf("Failed to install plugin '%s': %v", ref, err), nil
	}

	return fmt.Sprintf(
		"Installed plugin '%s' to %s scope.\n\nRun /reload-plugins to refresh skills, agents, MCP servers, and hooks.",
		ref,
		scope,
	), nil
}

// pluginHandleMarketplace dispatches /plugin marketplace subcommands.
func pluginHandleMarketplace(ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) == 0 {
		return strings.Join([]string{
			"Usage: /plugin marketplace <subcommand>",
			"",
			"Subcommands:",
			"  /plugin marketplace list",
			"  /plugin marketplace add <owner/repo|path> [marketplace-id]",
			"  /plugin marketplace remove <marketplace-id>",
			"  /plugin marketplace sync <marketplace-id|all>",
		}, "\n"), nil
	}

	switch strings.ToLower(args[0]) {
	case "list":
		return pluginHandleMarketplaceList(cwd)
	case "add":
		return pluginHandleMarketplaceAdd(cwd, args[1:])
	case "remove":
		return pluginHandleMarketplaceRemove(cwd, args[1:])
	case "sync":
		return pluginHandleMarketplaceSync(ctx, cwd, args[1:])
	default:
		return fmt.Sprintf("Unknown marketplace subcommand: %s", args[0]), nil
	}
}

// pluginHandleMarketplaceList shows configured plugin marketplaces.
func pluginHandleMarketplaceList(cwd string) (string, error) {
	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	ids := manager.List()
	sort.Strings(ids)
	if len(ids) == 0 {
		return "No marketplaces configured.\n\nAdd one with: /plugin marketplace add <owner/repo|path> [marketplace-id]", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Marketplaces (%d configured):\n\n", len(ids))
	for _, id := range ids {
		entry, ok := manager.Get(id)
		if !ok {
			continue
		}

		source := entry.Source.Path
		if entry.Source.Source == "github" {
			source = entry.Source.Repo
		}
		fmt.Fprintf(&sb, "  %s (%s)\n", id, entry.Source.Source)
		if source != "" {
			fmt.Fprintf(&sb, "      %s\n", source)
		}
	}

	sb.WriteString("\nCommands:\n")
	sb.WriteString("  /plugin marketplace add <owner/repo|path> [marketplace-id]\n")
	sb.WriteString("  /plugin marketplace remove <marketplace-id>\n")
	sb.WriteString("  /plugin marketplace sync <marketplace-id|all>\n")
	return sb.String(), nil
}

// pluginHandleMarketplaceAdd registers a new marketplace source.
func pluginHandleMarketplaceAdd(cwd string, args []string) (string, error) {
	if len(args) == 0 || len(args) > 2 {
		return "Usage: /plugin marketplace add <owner/repo|path> [marketplace-id]", nil
	}

	source := strings.TrimSpace(args[0])
	explicitID := ""
	if len(args) == 2 {
		explicitID = strings.TrimSpace(args[1])
	}

	id, normalizedSource, addFn, err := pluginParseMarketplaceSource(source, explicitID)
	if err != nil {
		return err.Error(), nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}
	if err := addFn(manager, id); err != nil {
		return fmt.Sprintf("Failed to add marketplace: %v", err), nil
	}

	return fmt.Sprintf(
		"Added marketplace '%s'.\n\nSource: %s\nInstall plugins with: /plugin install <plugin>@%s",
		id,
		normalizedSource,
		id,
	), nil
}

// pluginHandleMarketplaceRemove removes a configured marketplace.
func pluginHandleMarketplaceRemove(cwd string, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /plugin marketplace remove <marketplace-id>", nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	id := strings.TrimSpace(args[0])
	if _, ok := manager.Get(id); !ok {
		return fmt.Sprintf("Marketplace not found: %s", id), nil
	}
	if err := manager.Remove(id); err != nil {
		return fmt.Sprintf("Failed to remove marketplace '%s': %v", id, err), nil
	}

	return fmt.Sprintf("Removed marketplace '%s'.", id), nil
}

// pluginHandleMarketplaceSync updates one or all configured marketplaces.
func pluginHandleMarketplaceSync(ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /plugin marketplace sync <marketplace-id|all>", nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	target := strings.TrimSpace(args[0])
	if target == "all" {
		errs := manager.SyncAll(ctx)
		if len(errs) == 0 {
			return "Synced all marketplaces.", nil
		}
		var sb strings.Builder
		sb.WriteString("Failed to sync some marketplaces:\n")
		for _, err := range errs {
			fmt.Fprintf(&sb, "  - %v\n", err)
		}
		return strings.TrimRight(sb.String(), "\n"), nil
	}

	if err := manager.Sync(ctx, target); err != nil {
		return fmt.Sprintf("Failed to sync marketplace '%s': %v", target, err), nil
	}
	return fmt.Sprintf("Synced marketplace '%s'.", target), nil
}

func pluginParseScopeArg(raw string) (coreplugin.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(coreplugin.ScopeUser):
		return coreplugin.ScopeUser, nil
	case string(coreplugin.ScopeProject):
		return coreplugin.ScopeProject, nil
	case string(coreplugin.ScopeLocal):
		return coreplugin.ScopeLocal, nil
	default:
		return "", fmt.Errorf("invalid scope: %s (expected user, project, or local)", raw)
	}
}

func pluginParseMarketplaceSource(source, explicitID string) (id, normalizedSource string, addFn func(*coreplugin.MarketplaceManager, string) error, err error) {
	source = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(source, "]"), "["))
	if source == "" {
		return "", "", nil, fmt.Errorf("usage: /plugin marketplace add <owner/repo|path> [marketplace-id]")
	}

	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		absPath, err := pluginExpandMarketplacePath(source)
		if err != nil {
			return "", "", nil, err
		}
		id = explicitID
		if id == "" {
			id = filepath.Base(absPath)
		}
		return id, absPath, func(manager *coreplugin.MarketplaceManager, id string) error {
			return manager.AddDirectory(id, absPath)
		}, nil
	}

	if strings.HasPrefix(source, "https://github.com/") {
		repo := strings.TrimPrefix(source, "https://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		repo = strings.TrimSuffix(repo, "/")
		return pluginParseGitHubMarketplace(repo, explicitID)
	}

	if strings.Contains(source, "/") && !strings.Contains(source, "://") {
		return pluginParseGitHubMarketplace(source, explicitID)
	}

	return "", "", nil, fmt.Errorf("invalid source format. Use owner/repo, https://github.com/owner/repo, or ./path")
}

func pluginParseGitHubMarketplace(repo, explicitID string) (id, normalizedSource string, addFn func(*coreplugin.MarketplaceManager, string) error, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) < 2 {
		return "", "", nil, fmt.Errorf("invalid GitHub repository: %s", repo)
	}

	id = explicitID
	if id == "" {
		id = parts[len(parts)-1]
	}

	return id, repo, func(manager *coreplugin.MarketplaceManager, id string) error {
		return manager.AddGitHub(id, repo)
	}, nil
}

func pluginExpandMarketplacePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}

// ── Load ─────────────────────────────────────────────────────────────────────

// EnterSelect enters plugin selection mode
func (s *PluginSelector) EnterSelect(width, height int) error {
	s.active = true
	s.width = width
	s.height = height
	s.resetListState()
	s.resetDetailState()
	s.resetBrowseState()
	s.resetInputState()
	s.resetLoadingState()
	s.clearMessage()

	s.maxVisible = max(4, height-14)

	if err := s.marketplaceManager.Load(); err != nil {
		s.setError(fmt.Sprintf("Failed to load marketplaces: %v", err))
	}
	_ = s.installer.LoadMarketplaces() // Non-fatal

	s.refreshCurrentTab()
	return nil
}

// refreshCurrentTab refreshes data for the current tab
func (s *PluginSelector) refreshCurrentTab() {
	switch s.activeTab {
	case pluginTabInstalled:
		s.refreshInstalledPlugins()
	case pluginTabDiscover:
		s.refreshDiscoverPlugins()
	case pluginTabMarketplaces:
		s.refreshMarketplaces()
	}
	s.updateFilter()
}

// refreshInstalledPlugins loads installed plugins grouped by scope
func (s *PluginSelector) refreshInstalledPlugins() {
	plugins := s.registry.List()
	s.installedPlugins = make(map[coreplugin.Scope][]pluginItem)

	for _, p := range plugins {
		item := pluginItem{
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

	s.installedFlatList = []pluginItem{}
	for _, scope := range s.installedScopes {
		s.installedFlatList = append(s.installedFlatList, s.installedPlugins[scope]...)
	}
}

// refreshDiscoverPlugins loads available plugins from all marketplaces
func (s *PluginSelector) refreshDiscoverPlugins() {
	s.discoverPlugins = []pluginDiscoverItem{}
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
func (s *PluginSelector) refreshMarketplaces() {
	s.marketplaces = []pluginMarketplaceItem{}

	installedCounts := make(map[string]int)
	for _, p := range s.registry.List() {
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

		item := pluginMarketplaceItem{
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
func (s *PluginSelector) getInstalledNames() map[string]bool {
	names := make(map[string]bool)
	for _, p := range s.registry.List() {
		names[p.FullName()] = true
		names[p.Name()] = true
	}
	return names
}

// newDiscoverItem creates a pluginDiscoverItem with installed status set.
func (s *PluginSelector) newDiscoverItem(name, marketplaceID string, installed map[string]bool) pluginDiscoverItem {
	fullName := name + "@" + marketplaceID
	return pluginDiscoverItem{
		Name:        name,
		Marketplace: marketplaceID,
		Installed:   installed[fullName] || installed[name],
	}
}

// enrichDiscoverItem loads manifest details into an item.
func (s *PluginSelector) enrichDiscoverItem(item *pluginDiscoverItem) {
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
func (s *PluginSelector) refreshAndUpdateView() {
	s.refreshCurrentTab()
	if s.level == pluginLevelDetail && s.detailPlugin != nil {
		s.refreshDetailView()
	}
}

// refreshDetailView updates the detail plugin and actions after a state change
func (s *PluginSelector) refreshDetailView() {
	if s.detailPlugin == nil {
		return
	}
	name := s.detailPlugin.FullName
	for _, item := range s.filteredItems {
		if p, ok := item.(pluginItem); ok && p.FullName == name {
			s.detailPlugin = &p
			s.actions = s.buildInstalledActions(p)
			s.clampActionIdx()
			return
		}
	}
	s.goBack()
}

func (s *PluginSelector) clampActionIdx() {
	if s.actionIdx >= len(s.actions) {
		s.actionIdx = len(s.actions) - 1
	}
	if s.actionIdx < 0 {
		s.actionIdx = 0
	}
}

// addMarketplace adds a new marketplace
func (s *PluginSelector) addMarketplace() tea.Cmd {
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

	s.level = pluginLevelTabList
	s.addMarketplaceInput = ""
	s.refreshMarketplaces()

	return s.syncMarketplace(id)
}

// ── Reset ────────────────────────────────────────────────────────────────────

func (s *PluginSelector) resetListState() {
	s.level = pluginLevelTabList
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.parentIdx = 0
}

func (s *PluginSelector) resetDetailState() {
	s.detailPlugin = nil
	s.detailDiscover = nil
	s.detailMarketplace = nil
	s.actions = nil
	s.actionIdx = 0
	s.detailScroll = 0
}

func (s *PluginSelector) resetBrowseState() {
	s.browseMarketplaceID = ""
	s.browsePlugins = nil
}

func (s *PluginSelector) resetInputState() {
	s.searchQuery = ""
	s.filteredItems = nil
	s.addMarketplaceInput = ""
	s.addDialogCursor = 0
}

func (s *PluginSelector) resetLoadingState() {
	s.isLoading = false
	s.loadingMsg = ""
}

// Cancel cancels the selector and clears transient UI state.
func (s *PluginSelector) Cancel() {
	s.active = false
	s.resetListState()
	s.resetDetailState()
	s.resetBrowseState()
	s.resetInputState()
	s.resetLoadingState()
	s.clearMessage()
}
func (s *PluginSelector) Render() string {
	if !s.active {
		return ""
	}

	switch s.level {
	case pluginLevelDetail:
		if s.detailPlugin != nil {
			return s.renderInstalledDetail()
		}
		if s.detailDiscover != nil {
			return s.renderDiscoverDetail()
		}
		if s.detailMarketplace != nil {
			return s.renderMarketplaceDetail()
		}
	case pluginLevelAddMarketplace:
		return s.renderAddMarketplaceDialog()
	case pluginLevelBrowsePlugins:
		return s.renderBrowsePlugins()
	}

	return s.renderTabList()
}

func (s *PluginSelector) boxWidth() int {
	return max(60, s.width-6)
}

func (s *PluginSelector) boxHeight() int {
	return max(18, s.height-4)
}

func (s *PluginSelector) contentWidth() int {
	return s.boxWidth() - 4 // padding(1,2) takes 4 chars
}

func (s *PluginSelector) bodyHeight() int {
	return max(6, s.boxHeight()-10)
}

func (s *PluginSelector) sepLine() string {
	sepStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	return sepStyle.Render(strings.Repeat("─", s.contentWidth()-4))
}

// ── Full-width placement ──────────────────────────────────────────────────

func (s *PluginSelector) renderFullWidth(content string) string {
	box := lipgloss.NewStyle().
		Width(s.boxWidth()).
		Height(s.boxHeight()).
		Padding(1, 2).
		Render(content)
	return lipgloss.Place(s.width, s.height-2, lipgloss.Center, lipgloss.Top, box)
}

// ── Tabs ──────────────────────────────────────────────────────────────────

func (s *PluginSelector) renderTabs() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(kit.TabActiveFg).
		Background(kit.TabActiveBg).
		Bold(true).
		Padding(0, 2)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim).
		Padding(0, 2)

	tabs := []struct {
		name string
		tab  pluginTab
	}{
		{"Discover", pluginTabDiscover},
		{"Installed", pluginTabInstalled},
		{"Marketplaces", pluginTabMarketplaces},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == s.activeTab {
			parts = append(parts, activeStyle.Render(t.name))
		} else {
			parts = append(parts, inactiveStyle.Render(t.name))
		}
	}

	return strings.Join(parts, "  ")
}

// ── Search box ────────────────────────────────────────────────────────────

func (s *PluginSelector) renderSearchBox(sb *strings.Builder) {
	innerWidth := max(20, s.contentWidth()-4)

	var text string
	if s.searchQuery != "" {
		pos, total := s.getItemCount()
		text = fmt.Sprintf(" 🔍 %s▏ (%d/%d)", s.searchQuery, pos, total)
	} else {
		switch s.activeTab {
		case pluginTabDiscover:
			text = " 🔍 Type to filter plugins..."
		case pluginTabInstalled:
			text = " 🔍 Type to filter installed..."
		case pluginTabMarketplaces:
			text = " 🔍 Type to filter marketplaces..."
		}
	}

	textFg := kit.CurrentTheme.TextDim
	if s.searchQuery != "" {
		textFg = kit.CurrentTheme.Text
	}

	sb.WriteString(lipgloss.NewStyle().
		Foreground(textFg).
		Background(kit.SearchBg).
		Padding(0, 1).
		Width(innerWidth).
		Render(text))
}

// ── Tab list (main view) ──────────────────────────────────────────────────

func (s *PluginSelector) renderTabList() string {
	var sb strings.Builder

	// Separator above tabs
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")

	// Tab header
	sb.WriteString(s.renderTabs())
	sb.WriteString("\n\n")

	// Search box
	s.renderSearchBox(&sb)
	sb.WriteString("\n\n")

	// Tab content
	var body strings.Builder
	switch s.activeTab {
	case pluginTabInstalled:
		s.renderInstalledList(&body)
	case pluginTabDiscover:
		s.renderDiscoverList(&body)
	case pluginTabMarketplaces:
		s.renderMarketplacesList(&body)
	}
	sb.WriteString(s.renderViewport(body.String(), 0))

	// Footer
	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, s.getTabHint())

	return s.renderFullWidth(sb.String())
}

func (s *PluginSelector) getItemCount() (int, int) {
	total := len(s.filteredItems)
	if s.activeTab == pluginTabMarketplaces {
		total++
	}
	pos := s.selectedIdx + 1
	if total == 0 {
		pos = 0
	}
	return pos, total
}

func (s *PluginSelector) getTabHint() string {
	switch s.activeTab {
	case pluginTabInstalled:
		return "←/→ tabs · ↑/↓ navigate · space toggle · enter details · esc close"
	case pluginTabDiscover:
		return "←/→ tabs · ↑/↓ navigate · enter details · esc close"
	case pluginTabMarketplaces:
		return "←/→ tabs · ↑/↓ navigate · u update · r remove · esc close"
	}
	return ""
}

// ── Installed list ────────────────────────────────────────────────────────

func (s *PluginSelector) renderInstalledList(sb *strings.Builder) {
	dimStyle := kit.DimStyle()

	if len(s.filteredItems) == 0 {
		if len(s.installedFlatList) == 0 {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("No plugins installed"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.PaddingLeft(2).Render("Run: gen plugin install <name>@<marketplace>"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("No plugins match the filter"))
			sb.WriteString("\n")
		}
		return
	}

	visible := max(4, s.bodyHeight())
	endIdx := min(s.scrollOffset+visible, len(s.filteredItems))
	cw := s.contentWidth()

	if s.scrollOffset > 0 {
		sb.WriteString(dimStyle.PaddingLeft(2).Render("↑ more above"))
		sb.WriteString("\n")
	}

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(pluginItem)
		if !ok {
			continue
		}

		icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)

		nameStr := p.Name
		if p.Marketplace != "" {
			nameStr += dimStyle.Render(" · " + p.Marketplace)
		}

		line := fmt.Sprintf("%s %s", iconStyle.Render(icon), nameStr)

		if p.Description != "" {
			rawNameLen := len(p.Name)
			if p.Marketplace != "" {
				rawNameLen += 3 + len(p.Marketplace)
			}
			prefixLen := 6 + rawNameLen // cursor(2) + icon(1) + space(1) + padding(2)
			maxDescLen := cw - prefixLen - 2
			if maxDescLen > 20 {
				desc := kit.TruncateText(p.Description, maxDescLen)
				line += dimStyle.Render(" · " + desc)
			}
		}

		sb.WriteString(kit.RenderSelectableRow(line, i == s.selectedIdx))
		sb.WriteString("\n")
	}

	if endIdx < len(s.filteredItems) {
		sb.WriteString(dimStyle.PaddingLeft(2).Render("↓ more below"))
		sb.WriteString("\n")
	}
}

// ── Discover list ─────────────────────────────────────────────────────────

func (s *PluginSelector) renderDiscoverList(sb *strings.Builder) {
	dimStyle := kit.DimStyle()

	if len(s.filteredItems) == 0 {
		if len(s.discoverPlugins) == 0 {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("No plugins available"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.PaddingLeft(2).Render("Add a marketplace in the Marketplaces tab"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("No plugins match the filter"))
			sb.WriteString("\n")
		}
		return
	}

	maxItems := max(3, s.bodyHeight()/3)
	endIdx := min(s.scrollOffset+maxItems, len(s.filteredItems))

	if s.scrollOffset > 0 {
		sb.WriteString(dimStyle.PaddingLeft(2).Render("↑ more above"))
		sb.WriteString("\n")
	}

	cw := s.contentWidth()
	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(pluginDiscoverItem)
		if !ok {
			continue
		}

		icon := "○"
		iconStyle := dimStyle
		if p.Installed {
			icon = "●"
			iconStyle = kit.SelectorStatusConnected()
		}

		line := fmt.Sprintf("%s %s%s", iconStyle.Render(icon), p.Name, dimStyle.Render(" · "+p.Marketplace))
		sb.WriteString(kit.RenderSelectableRow(line, i == s.selectedIdx))
		sb.WriteString("\n")

		if p.Description != "" {
			maxDescLen := cw - 8
			if maxDescLen > 100 {
				maxDescLen = 100
			}
			if maxDescLen > 20 {
				desc := kit.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.PaddingLeft(6).Render(desc))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
	}

	remaining := len(s.filteredItems) - endIdx
	if remaining > 0 {
		sb.WriteString(dimStyle.PaddingLeft(2).Render(fmt.Sprintf("↓ %d more below", remaining)))
		sb.WriteString("\n")
	}
}

// ── Marketplaces list ─────────────────────────────────────────────────────

func (s *PluginSelector) renderMarketplacesList(sb *strings.Builder) {
	dimStyle := kit.DimStyle()
	addStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success).Bold(true)

	addLine := addStyle.Render("+ Add Marketplace")
	sb.WriteString(kit.RenderSelectableRow(addLine, s.selectedIdx == 0))
	sb.WriteString("\n")

	if len(s.filteredItems) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.PaddingLeft(2).Render("No marketplaces configured"))
		sb.WriteString("\n")
		return
	}

	sb.WriteString("\n")

	visible := max(4, s.bodyHeight()/2)
	endIdx := min(s.scrollOffset+visible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		m, ok := s.filteredItems[i].(pluginMarketplaceItem)
		if !ok {
			continue
		}

		displayIdx := i + 1
		official := ""
		if m.IsOfficial {
			official = " ★"
		}

		line := fmt.Sprintf("%s %s%s", kit.SelectorStatusConnected().Render("●"), m.ID, dimStyle.Render(official))
		sb.WriteString(kit.RenderSelectableRow(line, displayIdx == s.selectedIdx))
		sb.WriteString("\n")

		if displayIdx == s.selectedIdx {
			sb.WriteString(dimStyle.PaddingLeft(6).Render(m.Source))
			sb.WriteString("\n")
			stats := fmt.Sprintf("%d available · %d installed", m.Available, m.Installed)
			if m.LastUpdated != "" {
				stats += " · " + m.LastUpdated
			}
			sb.WriteString(dimStyle.PaddingLeft(6).Render(stats))
			sb.WriteString("\n")
		}
	}
}

// ── Detail views ──────────────────────────────────────────────────────────

func (s *PluginSelector) renderInstalledDetail() string {
	if s.detailPlugin == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailPlugin
	cw := s.contentWidth()

	dimStyle := kit.DimStyle()
	brightStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)
	labelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Width(12)

	// Header
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(kit.SelectorTitleStyle().Render("Plugin Details"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("> " + p.FullName))
	sb.WriteString("\n\n")

	icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)
	statusLabel := "Disabled"
	if p.Enabled {
		statusLabel = "Enabled"
	}
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Status:"), iconStyle.Render(icon+" "+statusLabel)))
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Scope:"), brightStyle.Render(string(p.Scope))))

	if p.Version != "" {
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Version:"), brightStyle.Render(p.Version)))
	}

	if p.Author != "" {
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Author:"), brightStyle.Render(p.Author)))
	}

	if p.Description != "" {
		sb.WriteString("\n")
		desc := kit.TruncateText(p.Description, cw-4)
		sb.WriteString("  " + dimStyle.Render(desc))
		sb.WriteString("\n")
	}

	components := pluginBuildComponentList(p)
	if len(components) > 0 {
		sb.WriteString("\n")
		compLabel := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Text).Bold(true)
		sb.WriteString("  " + compLabel.Render("Components"))
		sb.WriteString("\n")
		for _, c := range components {
			sb.WriteString("  " + dimStyle.Render("  • "+c))
			sb.WriteString("\n")
		}
	}

	if len(p.Errors) > 0 {
		sb.WriteString("\n")
		sb.WriteString("  " + kit.SelectorStatusError().Render("Errors"))
		sb.WriteString("\n")
		maxValueLen := cw - 8
		for _, err := range p.Errors {
			sb.WriteString("  " + kit.SelectorStatusError().Render("  • "+kit.TruncateText(err, maxValueLen)))
			sb.WriteString("\n")
		}
	}

	s.renderActions(&sb)

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, "↑/↓ scroll/actions · enter select · esc back")

	return s.renderFullWidth(sb.String())
}

func (s *PluginSelector) renderDiscoverDetail() string {
	if s.detailDiscover == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailDiscover
	cw := s.contentWidth()

	dimStyle := kit.DimStyle()
	brightStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)
	warnStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning)

	// Header
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(kit.SelectorTitleStyle().Render("Install Plugin"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("> " + p.Name + "@" + p.Marketplace))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(p.Name))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("from " + p.Marketplace))
	sb.WriteString("\n\n")

	if p.Description != "" {
		desc := kit.TruncateText(p.Description, cw-4)
		sb.WriteString("  " + dimStyle.Render(desc))
		sb.WriteString("\n\n")
	}

	if p.Author != "" {
		sb.WriteString("  " + dimStyle.Render("By: "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n\n")
	}

	sb.WriteString("  " + warnStyle.Render("⚠ Make sure you trust a plugin before installing"))
	sb.WriteString("\n")

	s.renderActions(&sb)

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, "↑/↓ scroll/actions · enter select · esc back")

	return s.renderFullWidth(sb.String())
}

func (s *PluginSelector) renderMarketplaceDetail() string {
	if s.detailMarketplace == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	m := s.detailMarketplace

	dimStyle := kit.DimStyle()
	brightStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)

	// Header
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(kit.SelectorTitleStyle().Render("Marketplace Details"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("> " + m.ID))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(m.ID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(m.Source))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("  %d available plugins", m.Available))
	sb.WriteString("\n")

	if m.Installed > 0 {
		sb.WriteString("\n")
		sb.WriteString("  " + dimStyle.Render(fmt.Sprintf("Installed (%d):", m.Installed)))
		sb.WriteString("\n")
		for _, p := range s.registry.List() {
			if idx := strings.Index(p.Source, "@"); idx != -1 && p.Source[idx+1:] == m.ID {
				sb.WriteString("    " + kit.SelectorStatusConnected().Render("●") + " " + p.Name())
				sb.WriteString("\n")
			}
		}
	}

	s.renderActions(&sb)

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, "↑/↓ scroll/actions · enter select · esc back")

	return s.renderFullWidth(sb.String())
}

// ── Add marketplace dialog ────────────────────────────────────────────────

func (s *PluginSelector) renderAddMarketplaceDialog() string {
	var sb strings.Builder
	cw := s.contentWidth()

	dimStyle := kit.DimStyle()
	brightStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)

	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(kit.SelectorTitleStyle().Render("Add Marketplace"))
	sb.WriteString("\n\n")

	sb.WriteString(dimStyle.Render("Enter marketplace source:"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Examples:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • https://github.com/owner/repo"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • owner/repo (GitHub shorthand)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • ./path/to/marketplace (local)"))
	sb.WriteString("\n\n")

	maxInputLen := cw - 6
	inputLine := s.addMarketplaceInput + "│"
	if len(inputLine) > maxInputLen {
		inputLine = "…" + inputLine[len(inputLine)-maxInputLen+1:]
	}
	sb.WriteString(brightStyle.Render("> " + inputLine))
	sb.WriteString("\n")

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, "enter add · esc cancel")

	return s.renderFullWidth(sb.String())
}

// ── Browse plugins ────────────────────────────────────────────────────────

func (s *PluginSelector) renderBrowsePlugins() string {
	var sb strings.Builder
	dimStyle := kit.DimStyle()
	brightStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextBright)

	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(kit.SelectorTitleStyle().Render("Browse Marketplace"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("> " + s.browseMarketplaceID))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(s.browseMarketplaceID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("%d available plugins", len(s.browsePlugins))))
	sb.WriteString("\n\n")

	cw := s.contentWidth()
	if len(s.browsePlugins) == 0 {
		sb.WriteString(dimStyle.PaddingLeft(2).Render("No plugins found"))
		sb.WriteString("\n")
	} else {
		visible := max(4, s.bodyHeight())
		endIdx := min(s.scrollOffset+visible, len(s.browsePlugins))

		if s.scrollOffset > 0 {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("↑ more above"))
			sb.WriteString("\n")
		}

		for i := s.scrollOffset; i < endIdx; i++ {
			p := s.browsePlugins[i]

			icon := "○"
			iconStyle := dimStyle
			if p.Installed {
				icon = "●"
				iconStyle = kit.SelectorStatusConnected()
			}

			line := fmt.Sprintf("%s %s", iconStyle.Render(icon), p.Name)
			sb.WriteString(kit.RenderSelectableRow(line, i == s.selectedIdx))
			sb.WriteString("\n")

			if p.Description != "" && i == s.selectedIdx {
				desc := kit.TruncateText(p.Description, cw-10)
				sb.WriteString(dimStyle.PaddingLeft(6).Render(desc))
				sb.WriteString("\n")
			}
		}

		if endIdx < len(s.browsePlugins) {
			sb.WriteString(dimStyle.PaddingLeft(2).Render("↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	s.renderFooter(&sb, "↑/↓ navigate · enter details · esc back")

	return s.renderFullWidth(sb.String())
}

// ── Actions ───────────────────────────────────────────────────────────────

func (s *PluginSelector) renderActions(sb *strings.Builder) {
	sb.WriteString("\n")
	accentStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.Primary).
		Bold(true).
		PaddingLeft(2)
	normalStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.Text).
		PaddingLeft(2)

	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(accentStyle.Render("❯ " + action.Label))
		} else {
			sb.WriteString(normalStyle.Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}
}

// ── Footer ────────────────────────────────────────────────────────────────

func (s *PluginSelector) renderFooter(sb *strings.Builder, hint string) {
	if s.isLoading {
		spinnerStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Accent)
		sb.WriteString(spinnerStyle.Render("  ◐ " + s.loadingMsg))
		sb.WriteString("\n")
	} else if s.lastMessage != "" {
		if s.isError {
			sb.WriteString(kit.SelectorStatusError().Render("  ⚠ " + s.lastMessage))
		} else {
			successStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success)
			sb.WriteString(successStyle.Render("  ✓ " + s.lastMessage))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(kit.DimStyle().Render(hint))
}

// ── Viewport ──────────────────────────────────────────────────────────────

func (s *PluginSelector) renderViewport(content string, scroll int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	visible := s.bodyHeight()
	if visible <= 0 {
		return ""
	}
	if scroll < 0 {
		scroll = 0
	}
	maxScroll := max(0, len(lines)-visible)
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := min(len(lines), scroll+visible)
	view := lines
	if len(lines) > 0 {
		view = lines[scroll:end]
	}

	if len(view) < visible {
		for len(view) < visible {
			view = append(view, "")
		}
	}

	return strings.Join(view, "\n") + "\n"
}

// ── Helpers ───────────────────────────────────────────────────────────────

func pluginStatusIconAndStyle(enabled bool) (string, lipgloss.Style) {
	if enabled {
		return "●", kit.SelectorStatusConnected()
	}
	return "○", kit.SelectorStatusNone()
}

func pluginBuildComponentList(p *pluginItem) []string {
	type componentCount struct {
		icon  string
		name  string
		count int
	}
	counts := []componentCount{
		{"✦", "Skills", p.Skills},
		{"⚑", "Agents", p.Agents},
		{"⌘", "Commands", p.Commands},
		{"↪", "Hooks", p.Hooks},
		{"☉", "MCP Servers", p.MCP},
		{"⦾", "LSP Servers", p.LSP},
	}

	var result []string
	for _, c := range counts {
		if c.count > 0 {
			result = append(result, fmt.Sprintf("%s %s: %d", c.icon, c.name, c.count))
		}
	}
	return result
}

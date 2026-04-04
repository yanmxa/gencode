// Package provider provides the provider selector feature.
package provider

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

// SelectorType represents what kind of selection we're doing
type SelectorType int

const (
	SelectorTypeProvider SelectorType = iota
	SelectorTypeModel
)

// SelectionLevel represents the current selection level
type SelectionLevel int

const (
	LevelProvider SelectionLevel = iota
	LevelAuthMethod
)

// SelectorTab represents which tab is active in the provider selector
type SelectorTab int

const (
	TabLLM SelectorTab = iota
	TabSearch
)

// ProviderItem represents a provider in the first level
type ProviderItem struct {
	Provider    coreprovider.Provider
	DisplayName string
	AuthMethods []AuthMethodItem
}

// AuthMethodItem represents an auth method in the second level
type AuthMethodItem struct {
	Provider    coreprovider.Provider
	AuthMethod  coreprovider.AuthMethod
	DisplayName string
	Status      coreprovider.ProviderStatus
	EnvVars     []string
}

// ModelItem represents a model in the model selector
type ModelItem struct {
	ID               string
	Name             string
	DisplayName      string
	ProviderName     string
	AuthMethod       coreprovider.AuthMethod
	IsCurrent        bool
	InputTokenLimit  int
	OutputTokenLimit int
}

// SearchProviderItem represents a search provider in the selector
type SearchProviderItem struct {
	Name        search.ProviderName
	DisplayName string
	Status      string // "current", "available", "unavailable"
	RequiresKey bool
	EnvVars     []string
}

// Model holds the state for the interactive provider selector.
type Model struct {
	active       bool
	selectorType SelectorType
	level        SelectionLevel
	providers    []ProviderItem
	models       []ModelItem
	selectedIdx  int
	parentIdx    int // Selected provider index when in auth method level
	width        int
	height       int
	store        *coreprovider.Store

	// Tab support for provider selector
	tab             SelectorTab          // Current tab (LLM or Search)
	searchProviders []SearchProviderItem // Search providers list

	// Model selector scrolling and search
	scrollOffset   int         // First visible item index
	maxVisible     int         // Max items to show (default 10)
	searchQuery    string      // Fuzzy search query
	filteredModels []ModelItem // Filtered models based on search

	// Provider connection result (shown inline)
	lastConnectResult  string // Result message from last connection
	lastConnectAuthIdx int    // Index of auth method that was connected
	lastConnectSuccess bool   // Whether last connection was successful
}

// statusDisplayInfo contains display information for a provider status
type statusDisplayInfo struct {
	icon  string
	style lipgloss.Style
	desc  string
}

// statusDisplayMap maps provider status to display information
var statusDisplayMap = map[coreprovider.ProviderStatus]statusDisplayInfo{
	coreprovider.StatusConnected: {"●", shared.SelectorStatusConnected, ""},
	coreprovider.StatusAvailable: {"○", shared.SelectorStatusReady, "(available)"},
}

// getStatusDisplay returns the icon, style, and description for a provider status
func getStatusDisplay(status coreprovider.ProviderStatus) (icon string, style lipgloss.Style, desc string) {
	if info, ok := statusDisplayMap[status]; ok {
		return info.icon, info.style, info.desc
	}
	return "◌", shared.SelectorStatusNone, ""
}

// New creates a new provider selector Model.
func New() Model {
	return Model{
		active:      false,
		level:       LevelProvider,
		providers:   []ProviderItem{},
		selectedIdx: 0,
		parentIdx:   0,
		maxVisible:  10, // Default max visible items
	}
}

// SelectedMsg is sent when a provider is selected.
type SelectedMsg struct {
	Provider   coreprovider.Provider
	AuthMethod coreprovider.AuthMethod
}

// ModelSelectedMsg is sent when a model is selected.
type ModelSelectedMsg struct {
	ModelID      string
	ProviderName string
	AuthMethod   coreprovider.AuthMethod
}

// ConnectResultMsg is sent when inline connection completes.
type ConnectResultMsg struct {
	AuthIdx   int
	Success   bool
	Message   string
	NewStatus coreprovider.ProviderStatus
}

// SearchProviderSelectedMsg is sent when a search provider is selected.
type SearchProviderSelectedMsg struct {
	Provider search.ProviderName
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel cancels the selector.
func (s *Model) Cancel() {
	s.active = false
	s.providers = []ProviderItem{}
	s.selectedIdx = 0
	s.parentIdx = 0
	s.level = LevelProvider
	s.searchQuery = ""
	s.filteredModels = nil
	s.scrollOffset = 0
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible.
func (s *Model) ensureVisible() {
	if s.selectorType != SelectorTypeModel {
		return
	}
	// Scroll up if selection is above viewport
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	// Scroll down if selection is below viewport
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters models based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredModels = s.models
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredModels = make([]ModelItem, 0)
		for _, m := range s.models {
			// Fuzzy match: check if query chars appear in order
			if shared.FuzzyMatch(strings.ToLower(m.ID), query) ||
				shared.FuzzyMatch(strings.ToLower(m.DisplayName), query) ||
				shared.FuzzyMatch(strings.ToLower(m.ProviderName), query) {
				s.filteredModels = append(s.filteredModels, m)
			}
		}
	}
	// Reset selection and scroll
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// MoveUp moves the selection up.
func (s *Model) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down.
func (s *Model) MoveDown() {
	maxIdx := s.getMaxIndex()
	if s.selectedIdx < maxIdx {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// getMaxIndex returns the maximum selectable index for current level.
func (s *Model) getMaxIndex() int {
	// Model selector - use filtered models
	if s.selectorType == SelectorTypeModel {
		return len(s.filteredModels) - 1
	}

	// Provider selector - check which tab is active
	if s.tab == TabSearch {
		return len(s.searchProviders) - 1
	}

	// LLM Provider selector
	if s.level == LevelProvider {
		return len(s.providers) - 1
	}
	// Auth method level
	if s.parentIdx < len(s.providers) {
		return len(s.providers[s.parentIdx].AuthMethods) - 1
	}
	return 0
}

// Select handles selection and returns a command.
func (s *Model) Select() tea.Cmd {
	// Model selector - use filtered models
	if s.selectorType == SelectorTypeModel {
		if s.selectedIdx >= len(s.filteredModels) {
			return nil
		}
		selected := s.filteredModels[s.selectedIdx]
		s.active = false
		return func() tea.Msg {
			return ModelSelectedMsg{
				ModelID:      selected.ID,
				ProviderName: selected.ProviderName,
				AuthMethod:   selected.AuthMethod,
			}
		}
	}

	// Search provider tab
	if s.tab == TabSearch {
		if s.selectedIdx >= len(s.searchProviders) {
			return nil
		}
		selected := s.searchProviders[s.selectedIdx]

		// Check if available
		sp := search.CreateProvider(selected.Name)
		if !sp.IsAvailable() && selected.RequiresKey {
			// Show error message
			s.lastConnectResult = "Missing: " + strings.Join(selected.EnvVars, ", ")
			s.lastConnectAuthIdx = s.selectedIdx
			s.lastConnectSuccess = false
			return nil
		}

		// Save selection and update status
		if s.store != nil {
			_ = s.store.SetSearchProvider(string(selected.Name))
		}

		// Update status in the list
		for i := range s.searchProviders {
			if s.searchProviders[i].Status == "current" {
				s.searchProviders[i].Status = "available"
			}
		}
		s.searchProviders[s.selectedIdx].Status = "current"

		// Show success message
		s.lastConnectResult = "\u2713 Selected"
		s.lastConnectAuthIdx = s.selectedIdx
		s.lastConnectSuccess = true

		return func() tea.Msg {
			return SearchProviderSelectedMsg{
				Provider: selected.Name,
			}
		}
	}

	// LLM Provider selector
	if s.level == LevelProvider {
		// Move to auth method level
		if s.selectedIdx < len(s.providers) {
			s.parentIdx = s.selectedIdx
			s.level = LevelAuthMethod
			s.selectedIdx = 0
		}
		return nil
	}

	// Auth method level - toggle connect/disconnect
	if s.parentIdx >= len(s.providers) {
		return nil
	}
	authMethods := s.providers[s.parentIdx].AuthMethods
	if s.selectedIdx >= len(authMethods) {
		return nil
	}

	item := authMethods[s.selectedIdx]
	authIdx := s.selectedIdx

	if item.Status == coreprovider.StatusConnected {
		return s.disconnectAuthMethod(item, authMethods, authIdx)
	}

	return s.connectAuthMethod(item, authIdx)
}

// HandleConnectResult updates the selector state with connection result.
func (s *Model) HandleConnectResult(msg ConnectResultMsg) {
	s.lastConnectAuthIdx = msg.AuthIdx
	s.lastConnectResult = msg.Message
	s.lastConnectSuccess = msg.Success

	// Update the auth method status if successful
	if msg.Success && s.parentIdx < len(s.providers) {
		authMethods := s.providers[s.parentIdx].AuthMethods
		if msg.AuthIdx < len(authMethods) {
			authMethods[msg.AuthIdx].Status = msg.NewStatus
		}
	}
}

// GoBack goes back to the previous level.
func (s *Model) GoBack() bool {
	if s.level == LevelAuthMethod {
		s.level = LevelProvider
		s.selectedIdx = s.parentIdx
		// Clear connection result when going back
		s.lastConnectResult = ""
		return true
	}
	return false
}

// HandleKeypress handles a keypress and returns a command if selection is made.
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyTab:
		// Switch tabs in provider selector
		if s.selectorType == SelectorTypeProvider && s.level == LevelProvider {
			if s.tab == TabLLM {
				s.tab = TabSearch
			} else {
				s.tab = TabLLM
			}
			s.selectedIdx = 0
			s.lastConnectResult = "" // Clear any connection result
		}
		return nil
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter, tea.KeyRight:
		return s.Select()
	case tea.KeyLeft:
		s.GoBack()
		return nil
	case tea.KeyEsc:
		// In model selector with search, first clear search
		if s.selectorType == SelectorTypeModel && s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		if s.GoBack() {
			return nil
		}
		s.Cancel()
		return func() tea.Msg {
			return shared.DismissedMsg{}
		}
	case tea.KeyBackspace:
		// Handle backspace for search in model selector
		if s.selectorType == SelectorTypeModel && len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeySpace:
		// Handle space for search in model selector
		if s.selectorType == SelectorTypeModel {
			s.searchQuery += " "
			s.updateFilter()
			return nil
		}
	case tea.KeyRunes:
		// Handle text input for fuzzy search in model selector
		if s.selectorType == SelectorTypeModel {
			s.searchQuery += string(key.Runes)
			s.updateFilter()
			return nil
		}
	}

	// Handle j/k/h/l for vim-style navigation (only when not searching)
	if s.selectorType != SelectorTypeModel || s.searchQuery == "" {
		switch key.String() {
		case "j":
			s.MoveDown()
			return nil
		case "k":
			s.MoveUp()
			return nil
		case "l":
			return s.Select()
		case "h":
			s.GoBack()
			return nil
		}
	}

	return nil
}

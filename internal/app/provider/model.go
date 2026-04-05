// Package provider provides the provider selector feature.
package provider

import (
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

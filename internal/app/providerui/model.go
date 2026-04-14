// Package provider provides the unified model & provider selector feature.
package providerui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/ui/selector"
)

// Tab represents which tab is active in the selector.
type Tab int

const (
	TabModels    Tab = iota // model selection tab
	TabProviders            // provider management tab
)

// ItemKind represents a row type in the visible-items list.
type ItemKind int

const (
	ItemProviderHeader ItemKind = iota // non-selectable provider group header (Models tab)
	ItemModel                         // selectable model row (Models tab)
	ItemProvider                      // provider row (Providers tab)
	ItemAuthMethod                    // expanded auth-method sub-row (Providers tab)
)

// ListItem is a single row in the flattened visible-items list.
type ListItem struct {
	Kind        ItemKind
	Model       *ModelItem
	Provider    *ProviderItem
	AuthMethod  *AuthMethodItem
	ProviderIdx int // index into allProviders
}

// ProviderItem represents a provider with its auth methods.
type ProviderItem struct {
	Provider    coreprovider.Provider
	DisplayName string
	AuthMethods []AuthMethodItem
	Connected   bool // whether this provider has at least one connected auth method
}

// AuthMethodItem represents an auth method in the second level.
type AuthMethodItem struct {
	Provider    coreprovider.Provider
	AuthMethod  coreprovider.AuthMethod
	DisplayName string
	Status      coreprovider.ProviderStatus
	EnvVars     []string
}

// ModelItem represents a model in the selector.
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

// Model holds the state for the unified model & provider selector.
type Model struct {
	active bool
	width  int
	height int
	store  *coreprovider.Store

	// Tab
	activeTab Tab

	// Data
	connectedProviders []ProviderItem // providers with models (Models tab headers)
	allProviders       []ProviderItem // all providers (Providers tab)
	allModels          []ModelItem

	// Flattened visible-items list (rebuilt on state changes)
	visibleItems []ListItem
	selectedIdx  int
	scrollOffset int
	maxVisible   int

	// Providers tab: expanded provider
	expandedProviderIdx int // index into allProviders; -1 = none

	// Inline API-key input
	apiKeyInput       textinput.Model
	apiKeyActive      bool
	apiKeyEnvVar      string
	apiKeyProviderIdx int // index into allProviders
	apiKeyAuthIdx     int // index into that provider's AuthMethods

	// Model search / filter
	searchQuery    string
	filteredModels []ModelItem

	// Provider connection result (shown inline)
	lastConnectResult  string
	lastConnectAuthIdx int  // item index that triggered the connection
	lastConnectSuccess bool
}

// statusDisplayInfo contains display information for a provider status.
type statusDisplayInfo struct {
	icon  string
	style lipgloss.Style
	desc  string
}

// statusDisplayMap maps provider status to display information.
var statusDisplayMap = map[coreprovider.ProviderStatus]statusDisplayInfo{
	coreprovider.StatusConnected: {"●", selector.SelectorStatusConnected, ""},
	coreprovider.StatusAvailable: {"○", selector.SelectorStatusReady, "(available)"},
}

// getStatusDisplay returns the icon, style, and description for a provider status.
func getStatusDisplay(status coreprovider.ProviderStatus) (icon string, style lipgloss.Style, desc string) {
	if info, ok := statusDisplayMap[status]; ok {
		return info.icon, info.style, info.desc
	}
	return "◌", selector.SelectorStatusNone, ""
}

// New creates a new provider selector Model.
func New() Model {
	return Model{
		active:              false,
		selectedIdx:         0,
		maxVisible:          20,
		expandedProviderIdx: -1,
	}
}

// SelectedMsg is sent when a provider auth method is selected (for connection).
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

// ModelsLoadedMsg is sent when async model loading completes.
type ModelsLoadedMsg struct {
	Models []ModelItem
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

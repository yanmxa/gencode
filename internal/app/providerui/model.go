// Package provider provides the unified model & provider selector feature.
package providerui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	coreprovider "github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/app/selector"
)

// tab represents which tab is active in the selector.
type tab int

const (
	tabModels    tab = iota // model selection tab
	tabProviders            // provider management tab
)

// itemKind represents a row type in the visible-items list.
type itemKind int

const (
	itemProviderHeader itemKind = iota // non-selectable provider group header (Models tab)
	itemModel                        // selectable model row (Models tab)
	itemProvider                     // provider row (Providers tab)
	itemAuthMethod                    // expanded auth-method sub-row (Providers tab)
)

// listItem is a single row in the flattened visible-items list.
type listItem struct {
	Kind        itemKind
	Model       *modelItem
	Provider    *providerItem
	AuthMethod  *authMethodItem
	ProviderIdx int // index into allProviders
}

// providerItem represents a provider with its auth methods.
type providerItem struct {
	Provider    coreprovider.Provider
	DisplayName string
	AuthMethods []authMethodItem
	Connected   bool // whether this provider has at least one connected auth method
}

// authMethodItem represents an auth method in the second level.
type authMethodItem struct {
	Provider    coreprovider.Provider
	AuthMethod  coreprovider.AuthMethod
	DisplayName string
	Status      coreprovider.ProviderStatus
	EnvVars     []string
}

// modelItem represents a model in the selector.
type modelItem struct {
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
	activeTab tab

	// Data
	connectedProviders []providerItem // providers with models (Models tab headers)
	allProviders       []providerItem // all providers (Providers tab)
	allModels          []modelItem

	// Flattened visible-items list (rebuilt on state changes)
	visibleItems []listItem
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
	filteredModels []modelItem

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
	Models []modelItem
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

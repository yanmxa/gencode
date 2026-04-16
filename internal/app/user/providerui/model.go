// Package provider provides the unified model & provider selector feature.
package providerui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// tab represents which tab is active in the kit.
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
	Provider    llm.Name
	DisplayName string
	AuthMethods []authMethodItem
	Connected   bool // whether this provider has at least one connected auth method
}

// authMethodItem represents an auth method in the second level.
type authMethodItem struct {
	Provider    llm.Name
	AuthMethod  llm.AuthMethod
	DisplayName string
	Status      llm.Status
	EnvVars     []string
}

// modelItem represents a model in the kit.
type modelItem struct {
	ID               string
	Name             string
	DisplayName      string
	ProviderName     string
	AuthMethod       llm.AuthMethod
	IsCurrent        bool
	InputTokenLimit  int
	OutputTokenLimit int
}

// Model holds the state for the unified model & provider kit.
type Model struct {
	active bool
	width  int
	height int
	store  *llm.Store

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
var statusDisplayMap = map[llm.Status]statusDisplayInfo{
	llm.StatusConnected: {"●", kit.SelectorStatusConnected(), ""},
	llm.StatusAvailable: {"○", kit.SelectorStatusReady(), "(available)"},
}

// getStatusDisplay returns the icon, style, and description for a provider status.
func getStatusDisplay(status llm.Status) (icon string, style lipgloss.Style, desc string) {
	if info, ok := statusDisplayMap[status]; ok {
		return info.icon, info.style, info.desc
	}
	return "◌", kit.SelectorStatusNone(), ""
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
	Provider   llm.Name
	AuthMethod llm.AuthMethod
}

// ModelSelectedMsg is sent when a model is selected.
type ModelSelectedMsg struct {
	ModelID      string
	ProviderName string
	AuthMethod   llm.AuthMethod
}

// ConnectResultMsg is sent when inline connection completes.
type ConnectResultMsg struct {
	AuthIdx   int
	Success   bool
	Message   string
	NewStatus llm.Status
}

// ModelsLoadedMsg is sent when async model loading completes.
type ModelsLoadedMsg struct {
	Models []modelItem
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

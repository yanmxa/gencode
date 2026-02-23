package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
	"github.com/yanmxa/gencode/internal/tui/theme"
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
	Provider    provider.Provider
	DisplayName string
	AuthMethods []AuthMethodItem
}

// AuthMethodItem represents an auth method in the second level
type AuthMethodItem struct {
	Provider    provider.Provider
	AuthMethod  provider.AuthMethod
	DisplayName string
	Status      provider.ProviderStatus
	EnvVars     []string
}

// ModelItem represents a model in the model selector
type ModelItem struct {
	ID               string
	Name             string
	DisplayName      string
	ProviderName     string
	AuthMethod       provider.AuthMethod
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

// SelectorState holds the state for the interactive selector
type SelectorState struct {
	active       bool
	selectorType SelectorType
	level        SelectionLevel
	providers    []ProviderItem
	models       []ModelItem
	selectedIdx  int
	parentIdx    int // Selected provider index when in auth method level
	width        int
	height       int
	store        *provider.Store

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
var statusDisplayMap = map[provider.ProviderStatus]statusDisplayInfo{
	provider.StatusConnected: {"●", SelectorStatusConnected, ""},
	provider.StatusAvailable: {"○", SelectorStatusReady, "(available)"},
}

// getStatusDisplay returns the icon, style, and description for a provider status
func getStatusDisplay(status provider.ProviderStatus) (icon string, style lipgloss.Style, desc string) {
	if info, ok := statusDisplayMap[status]; ok {
		return info.icon, info.style, info.desc
	}
	return "◌", SelectorStatusNone, ""
}

// NewSelectorState creates a new SelectorState
func NewSelectorState() SelectorState {
	return SelectorState{
		active:      false,
		level:       LevelProvider,
		providers:   []ProviderItem{},
		selectedIdx: 0,
		parentIdx:   0,
		maxVisible:  10, // Default max visible items
	}
}

// ProviderSelectedMsg is sent when a provider is selected
type ProviderSelectedMsg struct {
	Provider   provider.Provider
	AuthMethod provider.AuthMethod
}

// ModelSelectedMsg is sent when a model is selected
type ModelSelectedMsg struct {
	ModelID      string
	ProviderName string
	AuthMethod   provider.AuthMethod
}

// SelectorCancelledMsg is sent when the selector is cancelled
type SelectorCancelledMsg struct{}

// ProviderConnectResultMsg is sent when inline connection completes
type ProviderConnectResultMsg struct {
	AuthIdx   int
	Success   bool
	Message   string
	NewStatus provider.ProviderStatus
}

// SearchProviderSelectedMsg is sent when a search provider is selected
type SearchProviderSelectedMsg struct {
	Provider search.ProviderName
}

// EnterProviderSelect enters provider selection mode
func (s *SelectorState) EnterProviderSelect(width, height int) error {
	store, err := provider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}
	s.store = store

	providersWithStatus := provider.GetProvidersWithStatus(store)

	// Build provider list with auth methods
	s.providers = []ProviderItem{}

	providerOrder := []provider.Provider{
		provider.ProviderAnthropic,
		provider.ProviderOpenAI,
		provider.ProviderGoogle,
		provider.ProviderMoonshot,
	}
	displayNames := map[provider.Provider]string{
		provider.ProviderAnthropic: "Anthropic",
		provider.ProviderOpenAI:    "OpenAI",
		provider.ProviderGoogle:    "Google",
		provider.ProviderMoonshot:  "Moonshot",
	}

	for _, p := range providerOrder {
		infos, ok := providersWithStatus[p]
		if !ok || len(infos) == 0 {
			continue
		}

		providerItem := ProviderItem{
			Provider:    p,
			DisplayName: displayNames[p],
			AuthMethods: make([]AuthMethodItem, 0, len(infos)),
		}

		for _, info := range infos {
			providerItem.AuthMethods = append(providerItem.AuthMethods, AuthMethodItem{
				Provider:    info.Meta.Provider,
				AuthMethod:  info.Meta.AuthMethod,
				DisplayName: info.Meta.DisplayName,
				Status:      info.Status,
				EnvVars:     info.Meta.EnvVars,
			})
		}

		s.providers = append(s.providers, providerItem)
	}

	s.active = true
	s.selectorType = SelectorTypeProvider
	s.level = LevelProvider
	s.tab = TabLLM
	s.selectedIdx = 0
	s.parentIdx = 0
	s.width = width
	s.height = height

	// Load search providers
	s.loadSearchProviders()

	return nil
}

// loadSearchProviders loads the search provider list
func (s *SelectorState) loadSearchProviders() {
	currentProvider := ""
	if s.store != nil {
		currentProvider = s.store.GetSearchProvider()
	}
	if currentProvider == "" {
		currentProvider = string(search.ProviderExa) // Default
	}

	s.searchProviders = []SearchProviderItem{}
	for _, meta := range search.AllProviders() {
		sp := search.CreateProvider(meta.Name)
		status := "unavailable"
		if sp.IsAvailable() {
			status = "available"
		}
		if string(meta.Name) == currentProvider {
			status = "current"
		}

		s.searchProviders = append(s.searchProviders, SearchProviderItem{
			Name:        meta.Name,
			DisplayName: meta.DisplayName,
			Status:      status,
			RequiresKey: meta.RequiresAPIKey,
			EnvVars:     meta.EnvVars,
		})
	}
}

// EnterModelSelect enters model selection mode
func (s *SelectorState) EnterModelSelect(ctx context.Context, width, height int) error {
	store, err := provider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}
	s.store = store

	current := store.GetCurrentModel()
	var currentModelID string
	if current != nil {
		currentModelID = current.ModelID
	}

	// Get models from connected providers or cache
	s.models = []ModelItem{}

	allModels := store.GetAllCachedModels()

	if len(allModels) == 0 {
		// No cached models, get from connected providers
		connections := store.GetConnections()
		for providerName, conn := range connections {
			p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod)
			if err != nil {
				continue
			}

			models, err := p.ListModels(ctx)
			if err != nil {
				continue
			}

			// Cache the models
			prov := provider.Provider(providerName)
			_ = store.CacheModels(prov, conn.AuthMethod, models)

			for _, mdl := range models {
				s.models = append(s.models, ModelItem{
					ID:               mdl.ID,
					Name:             mdl.Name,
					DisplayName:      mdl.DisplayName,
					ProviderName:     providerName,
					AuthMethod:       conn.AuthMethod,
					IsCurrent:        mdl.ID == currentModelID,
					InputTokenLimit:  mdl.InputTokenLimit,
					OutputTokenLimit: mdl.OutputTokenLimit,
				})
			}
		}
	} else {
		// Use cached models
		for key, models := range allModels {
			parts := strings.SplitN(key, ":", 2)
			providerName := key
			var authMethod provider.AuthMethod
			if len(parts) >= 2 {
				providerName = parts[0]
				authMethod = provider.AuthMethod(parts[1])
			}

			for _, mdl := range models {
				s.models = append(s.models, ModelItem{
					ID:               mdl.ID,
					Name:             mdl.Name,
					DisplayName:      mdl.DisplayName,
					ProviderName:     providerName,
					AuthMethod:       authMethod,
					IsCurrent:        mdl.ID == currentModelID,
					InputTokenLimit:  mdl.InputTokenLimit,
					OutputTokenLimit: mdl.OutputTokenLimit,
				})
			}
		}
	}

	if len(s.models) == 0 {
		return fmt.Errorf("no models available. Connect to a provider first with /provider")
	}

	// Sort models: current model first, then by provider name
	sortModelsWithCurrentFirst(s.models)

	// Initialize filtered models (no filter initially)
	s.filteredModels = s.models
	s.searchQuery = ""
	s.scrollOffset = 0

	// Current model is now at index 0 (if exists)
	s.selectedIdx = 0

	// Ensure selected item is visible
	s.ensureVisible()

	s.active = true
	s.selectorType = SelectorTypeModel
	s.width = width
	s.height = height

	return nil
}

// IsActive returns whether the selector is active
func (s *SelectorState) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *SelectorState) Cancel() {
	s.active = false
	s.providers = []ProviderItem{}
	s.selectedIdx = 0
	s.parentIdx = 0
	s.level = LevelProvider
	s.searchQuery = ""
	s.filteredModels = nil
	s.scrollOffset = 0
}

// ensureVisible adjusts scrollOffset to keep selectedIdx visible
func (s *SelectorState) ensureVisible() {
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

// updateFilter filters models based on search query (fuzzy match)
func (s *SelectorState) updateFilter() {
	if s.searchQuery == "" {
		s.filteredModels = s.models
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredModels = make([]ModelItem, 0)
		for _, m := range s.models {
			// Fuzzy match: check if query chars appear in order
			if FuzzyMatch(strings.ToLower(m.ID), query) ||
				FuzzyMatch(strings.ToLower(m.DisplayName), query) ||
				FuzzyMatch(strings.ToLower(m.ProviderName), query) {
				s.filteredModels = append(s.filteredModels, m)
			}
		}
	}
	// Reset selection and scroll
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// sortModelsWithCurrentFirst sorts models with current model first, then by provider name
func sortModelsWithCurrentFirst(models []ModelItem) {
	sort.SliceStable(models, func(i, j int) bool {
		// Current model always comes first
		if models[i].IsCurrent && !models[j].IsCurrent {
			return true
		}
		if !models[i].IsCurrent && models[j].IsCurrent {
			return false
		}
		// Then sort by provider name
		return models[i].ProviderName < models[j].ProviderName
	})
}

// MoveUp moves the selection up
func (s *SelectorState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down
func (s *SelectorState) MoveDown() {
	maxIdx := s.getMaxIndex()
	if s.selectedIdx < maxIdx {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// getMaxIndex returns the maximum selectable index for current level
func (s *SelectorState) getMaxIndex() int {
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

// Select handles selection and returns a command
func (s *SelectorState) Select() tea.Cmd {
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
		s.lastConnectResult = "✓ Selected"
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

	// If already connected, disconnect
	if item.Status == provider.StatusConnected {
		store, _ := provider.NewStore()
		if store != nil {
			_ = store.Disconnect(item.Provider)
		}
		s.lastConnectResult = "✓ Disconnected"
		s.lastConnectAuthIdx = authIdx
		s.lastConnectSuccess = true
		authMethods[authIdx].Status = provider.StatusAvailable
		return nil
	}

	// Clear previous result
	s.lastConnectResult = "Connecting..."
	s.lastConnectAuthIdx = authIdx
	s.lastConnectSuccess = false

	// Return async command to connect
	return func() tea.Msg {
		ctx := context.Background()

		// Connect using the same logic as ConnectProvider
		meta, ok := provider.GetMeta(item.Provider, item.AuthMethod)
		if !ok {
			return ProviderConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Provider not found",
			}
		}

		if !provider.IsReady(meta) {
			return ProviderConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Missing env vars",
			}
		}

		llmProvider, err := provider.GetProvider(ctx, item.Provider, item.AuthMethod)
		if err != nil {
			return ProviderConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Failed: " + err.Error(),
			}
		}

		models, err := llmProvider.ListModels(ctx)

		// Save connection using a new store instance
		store, _ := provider.NewStore()
		if store != nil {
			if len(models) > 0 {
				_ = store.CacheModels(item.Provider, item.AuthMethod, models)
			}
			_ = store.Connect(item.Provider, item.AuthMethod)
		}

		if err != nil {
			// Models API unavailable - connected but using static models
			return ProviderConnectResultMsg{
				AuthIdx:   authIdx,
				Success:   true,
				Message:   fmt.Sprintf("⚠ %d models loaded (static)", len(models)),
				NewStatus: provider.StatusConnected,
			}
		}

		return ProviderConnectResultMsg{
			AuthIdx:   authIdx,
			Success:   true,
			Message:   fmt.Sprintf("✓ %d models loaded", len(models)),
			NewStatus: provider.StatusConnected,
		}
	}
}

// HandleConnectResult updates the selector state with connection result
func (s *SelectorState) HandleConnectResult(msg ProviderConnectResultMsg) {
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

// GoBack goes back to the previous level
func (s *SelectorState) GoBack() bool {
	if s.level == LevelAuthMethod {
		s.level = LevelProvider
		s.selectedIdx = s.parentIdx
		// Clear connection result when going back
		s.lastConnectResult = ""
		return true
	}
	return false
}

// HandleKeypress handles a keypress and returns a command if selection is made
func (s *SelectorState) HandleKeypress(key tea.KeyMsg) tea.Cmd {
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
			return SelectorCancelledMsg{}
		}
	case tea.KeyBackspace:
		// Handle backspace for search in model selector
		if s.selectorType == SelectorTypeModel && len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
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

// Render renders the selector
func (s *SelectorState) Render() string {
	if !s.active {
		return ""
	}

	// Model selector
	if s.selectorType == SelectorTypeModel {
		return s.renderModelSelector()
	}

	// Provider selector
	return s.renderProviderSelector()
}

// renderModelSelector renders the model selection UI
func (s *SelectorState) renderModelSelector() string {
	var sb strings.Builder

	// Title with count
	title := fmt.Sprintf("Select Model (%d/%d)", len(s.filteredModels), len(s.models))
	sb.WriteString(SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	// Search input box
	searchPrompt := "🔍 "
	if s.searchQuery == "" {
		sb.WriteString(SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "▏"))
	}
	sb.WriteString("\n\n")

	// Handle empty results
	if len(s.filteredModels) == 0 {
		sb.WriteString(SelectorHintStyle.Render("  No models match the filter"))
		sb.WriteString("\n")
	} else {
		// Calculate visible range
		endIdx := s.scrollOffset + s.maxVisible
		if endIdx > len(s.filteredModels) {
			endIdx = len(s.filteredModels)
		}

		// Show scroll up indicator
		if s.scrollOffset > 0 {
			sb.WriteString(SelectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		// Render visible models
		currentProvider := ""
		providerHeaderStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
		warningStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
		for i := s.scrollOffset; i < endIdx; i++ {
			m := s.filteredModels[i]

			// Show provider header when it changes
			if m.ProviderName != currentProvider {
				currentProvider = m.ProviderName
				// Capitalize first letter
				displayProvider := currentProvider
				if len(displayProvider) > 0 {
					displayProvider = strings.ToUpper(displayProvider[:1]) + displayProvider[1:]
				}
				sb.WriteString(providerHeaderStyle.Render(displayProvider) + "\n")
			}

			// Format: [*] Model Name
			indicator := "[ ]"
			indicatorStyle := SelectorStatusNone
			if m.IsCurrent {
				indicator = "[*]"
				indicatorStyle = SelectorStatusConnected
			}

			displayName := m.DisplayName
			if displayName == "" {
				displayName = m.Name
			}
			if displayName == "" {
				displayName = m.ID
			}

			// Add warning for models without token limits
			warning := ""
			if m.InputTokenLimit == 0 && m.OutputTokenLimit == 0 {
				warning = warningStyle.Render(" ⚠")
			}

			line := fmt.Sprintf("%s %s%s", indicatorStyle.Render(indicator), displayName, warning)

			if i == s.selectedIdx {
				sb.WriteString(SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		// Show scroll down indicator
		if endIdx < len(s.filteredModels) {
			sb.WriteString(SelectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(SelectorHintStyle.Render("↑/↓ navigate · Enter select · Esc clear/cancel"))
	sb.WriteString("\n")
	warningIcon := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning).Render("⚠")
	sb.WriteString(SelectorHintStyle.Render(warningIcon + " = No token limits (use /tokenlimit to set)"))

	// Wrap in border
	content := sb.String()
	box := SelectorBorderStyle.Width(CalculateBoxWidth(s.width)).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderProviderSelector renders the provider selection UI
func (s *SelectorState) renderProviderSelector() string {
	var sb strings.Builder

	// Tab header (only show at top level)
	if s.level == LevelProvider {
		sb.WriteString(s.renderTabHeader())
		sb.WriteString("\n\n")
	} else {
		// Title for auth method level
		sb.WriteString(SelectorTitleStyle.Render("Select Provider"))
		sb.WriteString("\n")
		// Breadcrumb for second level
		if s.parentIdx < len(s.providers) {
			breadcrumb := fmt.Sprintf("› %s", s.providers[s.parentIdx].DisplayName)
			sb.WriteString(SelectorBreadcrumbStyle.Render(breadcrumb))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Render based on tab and level
	if s.tab == TabSearch && s.level == LevelProvider {
		// Render search providers
		sb.WriteString(s.renderSearchProviders())
	} else if s.level == LevelProvider {
		// Render LLM provider list
		for i, p := range s.providers {
			// Count available auth methods
			availableCount := 0
			for _, am := range p.AuthMethods {
				if am.Status == provider.StatusConnected || am.Status == provider.StatusAvailable {
					availableCount++
				}
			}

			// Format: Provider Name (X available)
			statusText := ""
			if availableCount > 0 {
				statusText = fmt.Sprintf(" (%d available)", availableCount)
			}

			line := fmt.Sprintf("%s%s", p.DisplayName, SelectorStatusReady.Render(statusText))

			if i == s.selectedIdx {
				sb.WriteString(SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}
	} else {
		// Render auth method list
		if s.parentIdx < len(s.providers) {
			authMethods := s.providers[s.parentIdx].AuthMethods

			for i, am := range authMethods {
				// Format status icon and description
				statusIcon, statusStyle, statusDesc := getStatusDisplay(am.Status)

				line := fmt.Sprintf("%s %s %s",
					statusStyle.Render(statusIcon),
					am.DisplayName,
					SelectorStatusNone.Render(statusDesc),
				)

				if i == s.selectedIdx {
					sb.WriteString(SelectorSelectedStyle.Render("> " + line))
				} else {
					sb.WriteString(SelectorItemStyle.Render("  " + line))
				}
				sb.WriteString("\n")

				// Show connection result for this auth method
				if s.lastConnectResult != "" && i == s.lastConnectAuthIdx {
					sb.WriteString(SelectorItemStyle.Render("    " + s.renderConnectResult()))
					sb.WriteString("\n")
				}
			}
		}
	}

	sb.WriteString("\n")

	// Hint
	if s.level == LevelProvider {
		sb.WriteString(SelectorHintStyle.Render("Tab switch · ↑/↓ navigate · Enter select · Esc cancel"))
	} else {
		sb.WriteString(SelectorHintStyle.Render("↑/↓ navigate · Enter select · ←/Esc back"))
	}

	// Wrap in border
	content := sb.String()
	box := SelectorBorderStyle.Width(CalculateBoxWidth(s.width)).Render(content)

	// Center the box
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderTabHeader renders the tab header for the provider selector
func (s *SelectorState) renderTabHeader() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	var llmTab, searchTab string

	if s.tab == TabLLM {
		llmTab = activeStyle.Render("[LLM]")
		searchTab = inactiveStyle.Render(" Search ")
	} else {
		llmTab = inactiveStyle.Render(" LLM ")
		searchTab = activeStyle.Render("[Search]")
	}

	tabs := llmTab + "  " + searchTab

	// Calculate box width for centering
	boxWidth := CalculateBoxWidth(s.width)

	return lipgloss.PlaceHorizontal(boxWidth-4, lipgloss.Center, tabs)
}

// getSearchProviderStatus returns icon, style, and description for a search provider
func getSearchProviderStatus(status string, requiresKey bool) (icon string, style lipgloss.Style, desc string) {
	switch status {
	case "current":
		return "●", SelectorStatusConnected, ""
	case "available":
		return "○", SelectorStatusReady, ""
	default:
		if requiresKey {
			return "◌", SelectorStatusNone, "(no credentials)"
		}
		return "◌", SelectorStatusNone, ""
	}
}

// renderSearchProviders renders the search provider list
func (s *SelectorState) renderSearchProviders() string {
	var sb strings.Builder

	for i, sp := range s.searchProviders {
		statusIcon, statusStyle, statusDesc := getSearchProviderStatus(sp.Status, sp.RequiresKey)

		line := fmt.Sprintf("%s %s %s",
			statusStyle.Render(statusIcon),
			sp.DisplayName,
			SelectorStatusNone.Render(statusDesc),
		)

		if i == s.selectedIdx {
			sb.WriteString(SelectorSelectedStyle.Render("> " + line))
		} else {
			sb.WriteString(SelectorItemStyle.Render("  " + line))
		}
		sb.WriteString("\n")

		// Show result message for this provider
		if s.lastConnectResult != "" && i == s.lastConnectAuthIdx {
			sb.WriteString(SelectorItemStyle.Render("    " + s.renderConnectResult()))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// connectResultStyle returns the appropriate style for a connection result message.
func (s *SelectorState) connectResultStyle() lipgloss.Style {
	if !s.lastConnectSuccess {
		if s.lastConnectResult == "Connecting..." {
			return SelectorHintStyle
		}
		return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Error)
	}
	if strings.HasPrefix(s.lastConnectResult, "⚠") {
		return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
	}
	return SelectorStatusConnected
}

// renderConnectResult returns the styled result message for connection attempts.
func (s *SelectorState) renderConnectResult() string {
	return s.connectResultStyle().Render(s.lastConnectResult)
}

// ConnectProvider connects to the selected provider and verifies the connection
func (s *SelectorState) ConnectProvider(ctx context.Context, p provider.Provider, authMethod provider.AuthMethod) (string, error) {
	if s.store == nil {
		store, err := provider.NewStore()
		if err != nil {
			return "", fmt.Errorf("failed to load store: %w", err)
		}
		s.store = store
	}

	// Check if the provider is ready (has required env vars)
	meta, ok := provider.GetMeta(p, authMethod)
	if !ok {
		return "", fmt.Errorf("provider not found: %s:%s", p, authMethod)
	}

	if !provider.IsReady(meta) {
		missingVars := []string{}
		for _, envVar := range meta.EnvVars {
			if envVar == "" {
				continue
			}
			missingVars = append(missingVars, envVar)
		}
		return "", fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	// Try to get provider and list models to verify the connection
	llmProvider, err := provider.GetProvider(ctx, p, authMethod)
	if err != nil {
		return "", fmt.Errorf("failed to create provider: %w", err)
	}

	models, listErr := llmProvider.ListModels(ctx)

	// Cache models if available
	if len(models) > 0 {
		_ = s.store.CacheModels(p, authMethod, models)
	}

	// Save connection
	if err := s.store.Connect(p, authMethod); err != nil {
		return "", fmt.Errorf("failed to save connection: %w", err)
	}

	if listErr != nil {
		return fmt.Sprintf("Connected to %s via %s (⚠ %d static models)", meta.DisplayName, authMethod, len(models)), nil
	}

	return fmt.Sprintf("Connected to %s via %s (%d models)", meta.DisplayName, authMethod, len(models)), nil
}

// SetModel sets the current model
func (s *SelectorState) SetModel(modelID string, providerName string, authMethod provider.AuthMethod) (string, error) {
	if s.store == nil {
		store, err := provider.NewStore()
		if err != nil {
			return "", fmt.Errorf("failed to load store: %w", err)
		}
		s.store = store
	}

	if err := s.store.SetCurrentModel(modelID, provider.Provider(providerName), authMethod); err != nil {
		return "", fmt.Errorf("failed to set model: %w", err)
	}

	return fmt.Sprintf("Model set to: %s (%s)", modelID, providerName), nil
}

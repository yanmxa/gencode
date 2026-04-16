package providerui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/app/selector"
)

// providerOrder defines the display order for providers.
var providerOrder = []coreprovider.Provider{
	coreprovider.ProviderAnthropic,
	coreprovider.ProviderOpenAI,
	coreprovider.ProviderGoogle,
	coreprovider.ProviderMoonshot,
	coreprovider.ProviderAlibaba,
}

// providerDisplayNames maps provider to human-readable name.
var providerDisplayNames = map[coreprovider.Provider]string{
	coreprovider.ProviderAnthropic: "Anthropic",
	coreprovider.ProviderOpenAI:    "OpenAI",
	coreprovider.ProviderGoogle:    "Google",
	coreprovider.ProviderMoonshot:  "Moonshot",
	coreprovider.ProviderAlibaba:   "Alibaba",
}

// Enter opens the unified model & provider selector.
func (s *Model) Enter(ctx context.Context, width, height int) (tea.Cmd, error) {
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
	s.expandedProviderIdx = -1
	s.apiKeyActive = false
	s.active = true
	s.activeTab = tabModels
	s.width = width
	s.height = height

	cmd, err := s.loadProviderData()
	if err != nil {
		return nil, err
	}
	s.rebuildVisibleItems()
	return cmd, nil
}

// loadProviderData refreshes provider and model data from a fresh store.
// Does NOT reset UI state (tabs, selection, expansion) or call rebuildVisibleItems.
func (s *Model) loadProviderData() (tea.Cmd, error) {
	store, err := coreprovider.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}
	s.store = store

	providersWithStatus := coreprovider.GetProvidersWithStatus(store)

	s.connectedProviders = nil
	s.allProviders = nil

	for _, p := range providerOrder {
		infos, ok := providersWithStatus[p]
		if !ok || len(infos) == 0 {
			continue
		}

		item := providerItem{
			Provider:    p,
			DisplayName: providerDisplayNames[p],
			AuthMethods: make([]authMethodItem, 0, len(infos)),
		}

		connected := false
		for _, info := range infos {
			item.AuthMethods = append(item.AuthMethods, authMethodItem{
				Provider:    info.Meta.Provider,
				AuthMethod:  info.Meta.AuthMethod,
				DisplayName: info.Meta.DisplayName,
				Status:      info.Status,
				EnvVars:     info.Meta.EnvVars,
			})
			if info.Status == coreprovider.StatusConnected {
				connected = true
			}
		}

		item.Connected = connected
		s.allProviders = append(s.allProviders, item)
		if connected {
			s.connectedProviders = append(s.connectedProviders, item)
		}
	}

	current := store.GetCurrentModel()
	var currentModelID string
	if current != nil {
		currentModelID = current.ModelID
	}

	s.allModels = nil
	allCached := store.GetAllCachedModels()
	if len(allCached) == 0 {
		allCached = store.GetAllCachedModelsIncludeExpired()
	}

	var asyncCmd tea.Cmd
	if len(allCached) > 0 {
		s.loadModelsCached(allCached, currentModelID)
	} else {
		asyncCmd = s.loadModelsAsync(store, currentModelID)
	}

	s.ensureModelProvidersExist()
	s.sortConnectedProviders(currentModelID)

	return asyncCmd, nil
}

// ensureModelProvidersExist ensures every provider that has cached models
// is represented in connectedProviders (handles cases where registry doesn't
// have the provider registered but models exist in cache).
func (s *Model) ensureModelProvidersExist() {
	existing := make(map[string]bool)
	for _, cp := range s.connectedProviders {
		existing[string(cp.Provider)] = true
	}

	// Collect unique provider names from models
	seen := make(map[string]bool)
	for _, m := range s.allModels {
		if existing[m.ProviderName] || seen[m.ProviderName] {
			continue
		}
		seen[m.ProviderName] = true

		displayName := providerDisplayNames[coreprovider.Provider(m.ProviderName)]
		if displayName == "" {
			displayName = m.ProviderName
		}

		s.connectedProviders = append(s.connectedProviders, providerItem{
			Provider:    coreprovider.Provider(m.ProviderName),
			DisplayName: displayName,
			Connected:   true,
		})
	}
}

// loadModelsAsync returns a tea.Cmd that fetches models from all connected
// providers concurrently, sending a ModelsLoadedMsg when done.
func (s *Model) loadModelsAsync(store *coreprovider.Store, currentModelID string) tea.Cmd {
	connections := store.GetConnections()
	return func() tea.Msg {
		ctx := context.Background()

		type providerResult struct {
			providerName string
			authMethod   coreprovider.AuthMethod
			models       []coreprovider.ModelInfo
		}

		ch := make(chan providerResult, len(connections))
		var wg sync.WaitGroup

		for name, conn := range connections {
			wg.Add(1)
			go func(providerName string, authMethod coreprovider.AuthMethod) {
				defer wg.Done()
				p, err := coreprovider.GetProvider(ctx, coreprovider.Provider(providerName), authMethod)
				if err != nil {
					return
				}
				mdls, err := p.ListModels(ctx)
				if err != nil {
					return
				}
				ch <- providerResult{providerName, authMethod, mdls}
			}(name, conn.AuthMethod)
		}

		go func() { wg.Wait(); close(ch) }()

		var models []modelItem
		for r := range ch {
			prov := coreprovider.Provider(r.providerName)
			_ = store.CacheModels(prov, r.authMethod, r.models)

			for _, mdl := range r.models {
				models = append(models, modelItem{
					ID:               mdl.ID,
					Name:             mdl.Name,
					DisplayName:      mdl.DisplayName,
					ProviderName:     r.providerName,
					AuthMethod:       r.authMethod,
					IsCurrent:        mdl.ID == currentModelID,
					InputTokenLimit:  mdl.InputTokenLimit,
					OutputTokenLimit: mdl.OutputTokenLimit,
				})
			}
		}
		return ModelsLoadedMsg{Models: models}
	}
}

// HandleModelsLoaded updates the panel with asynchronously loaded models.
func (s *Model) HandleModelsLoaded(msg ModelsLoadedMsg) {
	s.allModels = msg.Models
	s.ensureModelProvidersExist()

	var currentModelID string
	if s.store != nil {
		if current := s.store.GetCurrentModel(); current != nil {
			currentModelID = current.ModelID
		}
	}
	s.sortConnectedProviders(currentModelID)
	s.rebuildVisibleItems()
}

// loadModelsCached loads models from the store cache.
func (s *Model) loadModelsCached(allCached map[string][]coreprovider.ModelInfo, currentModelID string) {
	for key, models := range allCached {
		parts := strings.SplitN(key, ":", 2)
		providerName := key
		var authMethod coreprovider.AuthMethod
		if len(parts) >= 2 {
			providerName = parts[0]
			authMethod = coreprovider.AuthMethod(parts[1])
		}

		for _, mdl := range models {
			s.allModels = append(s.allModels, modelItem{
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

// sortConnectedProviders sorts connected providers so that the current model's
// provider comes first, then alphabetical.
func (s *Model) sortConnectedProviders(currentModelID string) {
	if currentModelID == "" {
		return
	}
	var currentProvider string
	for _, m := range s.allModels {
		if m.ID == currentModelID {
			currentProvider = m.ProviderName
			break
		}
	}
	if currentProvider == "" {
		return
	}
	sort.SliceStable(s.connectedProviders, func(i, j int) bool {
		iMatch := string(s.connectedProviders[i].Provider) == currentProvider
		jMatch := string(s.connectedProviders[j].Provider) == currentProvider
		if iMatch != jMatch {
			return iMatch
		}
		return false
	})
}

// rebuildVisibleItems constructs the flat visible-items list from current state.
func (s *Model) rebuildVisibleItems() {
	s.visibleItems = nil

	switch s.activeTab {
	case tabModels:
		s.rebuildModelsTab()
	case tabProviders:
		s.rebuildProvidersTab()
	}

	s.clampSelection()
}

// rebuildModelsTab builds visible items for the Models tab.
func (s *Model) rebuildModelsTab() {
	s.applyFilter()

	// Group filtered models by provider
	providerModels := make(map[string][]modelItem)
	for i := range s.filteredModels {
		m := &s.filteredModels[i]
		providerModels[m.ProviderName] = append(providerModels[m.ProviderName], *m)
	}

	for i := range s.connectedProviders {
		cp := &s.connectedProviders[i]
		models := providerModels[string(cp.Provider)]
		if len(models) == 0 && s.searchQuery != "" {
			continue
		}

		s.visibleItems = append(s.visibleItems, listItem{
			Kind:        itemProviderHeader,
			Provider:    cp,
			ProviderIdx: i,
		})

		// Sort models: current first
		sort.SliceStable(models, func(a, b int) bool {
			return models[a].IsCurrent && !models[b].IsCurrent
		})

		for j := range models {
			s.visibleItems = append(s.visibleItems, listItem{
				Kind:        itemModel,
				Model:       &models[j],
				ProviderIdx: i,
			})
		}
	}
}

// rebuildProvidersTab builds visible items for the Providers tab.
func (s *Model) rebuildProvidersTab() {
	for i := range s.allProviders {
		p := &s.allProviders[i]

		// Apply search filter on provider name
		if s.searchQuery != "" {
			query := strings.ToLower(s.searchQuery)
			if !selector.FuzzyMatch(strings.ToLower(p.DisplayName), query) &&
				!selector.FuzzyMatch(strings.ToLower(string(p.Provider)), query) {
				continue
			}
		}

		s.visibleItems = append(s.visibleItems, listItem{
			Kind:        itemProvider,
			Provider:    p,
			ProviderIdx: i,
		})

		// Show expanded auth methods
		if s.expandedProviderIdx == i {
			for j := range p.AuthMethods {
				s.visibleItems = append(s.visibleItems, listItem{
					Kind:        itemAuthMethod,
					AuthMethod:  &p.AuthMethods[j],
					ProviderIdx: i,
				})
			}
		}
	}
}

func (s *Model) applyFilter() {
	if s.searchQuery == "" {
		s.filteredModels = s.allModels
		return
	}
	query := strings.ToLower(s.searchQuery)
	s.filteredModels = nil
	for _, m := range s.allModels {
		if selector.FuzzyMatch(strings.ToLower(m.ID), query) ||
			selector.FuzzyMatch(strings.ToLower(m.DisplayName), query) ||
			selector.FuzzyMatch(strings.ToLower(m.ProviderName), query) {
			s.filteredModels = append(s.filteredModels, m)
		}
	}
}

func (s *Model) clampSelection() {
	if len(s.visibleItems) == 0 {
		s.selectedIdx = 0
		return
	}
	if s.selectedIdx >= len(s.visibleItems) {
		s.selectedIdx = len(s.visibleItems) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
	// Skip non-selectable items forward
	if s.visibleItems[s.selectedIdx].Kind == itemProviderHeader {
		for s.selectedIdx < len(s.visibleItems)-1 {
			s.selectedIdx++
			if s.visibleItems[s.selectedIdx].Kind != itemProviderHeader {
				break
			}
		}
	}
}

// refreshAuthMethod re-fetches models for an already connected provider auth method.
func (s *Model) refreshAuthMethod(item authMethodItem, authIdx int) tea.Cmd {
	s.lastConnectResult = "Refreshing..."
	s.lastConnectAuthIdx = authIdx
	s.lastConnectSuccess = false

	return func() tea.Msg {
		ctx := context.Background()

		llmProvider, err := coreprovider.GetProvider(ctx, item.Provider, item.AuthMethod)
		if err != nil {
			return ConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Failed: " + err.Error(),
			}
		}

		models, err := llmProvider.ListModels(ctx)

		store, _ := coreprovider.NewStore()
		if store != nil && len(models) > 0 {
			_ = store.CacheModels(item.Provider, item.AuthMethod, models)
		}

		if err != nil {
			return ConnectResultMsg{
				AuthIdx:   authIdx,
				Success:   true,
				Message:   fmt.Sprintf("⚠ %d models (static)", len(models)),
				NewStatus: coreprovider.StatusConnected,
			}
		}

		return ConnectResultMsg{
			AuthIdx:   authIdx,
			Success:   true,
			Message:   fmt.Sprintf("✓ Refreshed: %d models", len(models)),
			NewStatus: coreprovider.StatusConnected,
		}
	}
}

// connectAuthMethod initiates an async connection to a provider auth method.
func (s *Model) connectAuthMethod(item authMethodItem, authIdx int) tea.Cmd {
	s.lastConnectResult = "Connecting..."
	s.lastConnectAuthIdx = authIdx
	s.lastConnectSuccess = false

	return func() tea.Msg {
		ctx := context.Background()

		meta, ok := coreprovider.GetMeta(item.Provider, item.AuthMethod)
		if !ok {
			return ConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Provider not found",
			}
		}

		if !coreprovider.IsReady(meta) {
			return ConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Missing env vars",
			}
		}

		llmProvider, err := coreprovider.GetProvider(ctx, item.Provider, item.AuthMethod)
		if err != nil {
			return ConnectResultMsg{
				AuthIdx: authIdx,
				Success: false,
				Message: "Failed: " + err.Error(),
			}
		}

		models, err := llmProvider.ListModels(ctx)

		store, _ := coreprovider.NewStore()
		if store != nil {
			if len(models) > 0 {
				_ = store.CacheModels(item.Provider, item.AuthMethod, models)
			}
			_ = store.Connect(item.Provider, item.AuthMethod)
		}

		if err != nil {
			return ConnectResultMsg{
				AuthIdx:   authIdx,
				Success:   true,
				Message:   fmt.Sprintf("⚠ %d models loaded (static)", len(models)),
				NewStatus: coreprovider.StatusConnected,
			}
		}

		return ConnectResultMsg{
			AuthIdx:   authIdx,
			Success:   true,
			Message:   fmt.Sprintf("✓ %d models loaded", len(models)),
			NewStatus: coreprovider.StatusConnected,
		}
	}
}

// HandleConnectResult updates the selector state with connection result.
func (s *Model) HandleConnectResult(msg ConnectResultMsg) tea.Cmd {
	s.lastConnectAuthIdx = msg.AuthIdx
	s.lastConnectResult = msg.Message
	s.lastConnectSuccess = msg.Success

	if !msg.Success {
		return nil
	}

	// Reload provider/model data, preserving UI state (tab, expansion, result).
	cmd, _ := s.loadProviderData()
	s.rebuildVisibleItems()
	return cmd
}

// ConnectProvider connects to a provider and verifies the connection.
func (s *Model) ConnectProvider(ctx context.Context, p coreprovider.Provider, authMethod coreprovider.AuthMethod) (string, error) {
	if s.store == nil {
		store, err := coreprovider.NewStore()
		if err != nil {
			return "", fmt.Errorf("failed to load store: %w", err)
		}
		s.store = store
	}

	meta, ok := coreprovider.GetMeta(p, authMethod)
	if !ok {
		return "", fmt.Errorf("provider not found: %s:%s", p, authMethod)
	}

	if !coreprovider.IsReady(meta) {
		missingVars := []string{}
		for _, envVar := range meta.EnvVars {
			if envVar == "" {
				continue
			}
			missingVars = append(missingVars, envVar)
		}
		return "", fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	llmProvider, err := coreprovider.GetProvider(ctx, p, authMethod)
	if err != nil {
		return "", fmt.Errorf("failed to create provider: %w", err)
	}

	models, listErr := llmProvider.ListModels(ctx)
	if len(models) > 0 {
		_ = s.store.CacheModels(p, authMethod, models)
	}

	if err := s.store.Connect(p, authMethod); err != nil {
		return "", fmt.Errorf("failed to save connection: %w", err)
	}

	if listErr != nil {
		return fmt.Sprintf("Connected to %s via %s (⚠ %d static models)", meta.DisplayName, authMethod, len(models)), nil
	}

	return fmt.Sprintf("Connected to %s via %s (%d models)", meta.DisplayName, authMethod, len(models)), nil
}

// SetModel sets the current model.
func (s *Model) SetModel(modelID string, providerName string, authMethod coreprovider.AuthMethod) (string, error) {
	if s.store == nil {
		store, err := coreprovider.NewStore()
		if err != nil {
			return "", fmt.Errorf("failed to load store: %w", err)
		}
		s.store = store
	}

	if err := s.store.SetCurrentModel(modelID, coreprovider.Provider(providerName), authMethod); err != nil {
		return "", fmt.Errorf("failed to set model: %w", err)
	}

	return fmt.Sprintf("Model set to: %s (%s)", modelID, providerName), nil
}

// initAPIKeyInput initializes the textinput for API key entry.
func (s *Model) initAPIKeyInput(envVar string) {
	ti := textinput.New()
	ti.Placeholder = envVar
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40
	ti.EchoMode = textinput.EchoPassword
	s.apiKeyInput = ti
	s.apiKeyActive = true
	s.apiKeyEnvVar = envVar
}

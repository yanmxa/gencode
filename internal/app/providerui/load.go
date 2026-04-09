package providerui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
)

// EnterProviderSelect enters provider selection mode.
func (s *Model) EnterProviderSelect(width, height int) error {
	store, err := coreprovider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}
	s.store = store
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
	s.models = nil
	s.filteredModels = nil

	providersWithStatus := coreprovider.GetProvidersWithStatus(store)

	// Build provider list with auth methods
	s.providers = []ProviderItem{}

	providerOrder := []coreprovider.Provider{
		coreprovider.ProviderAnthropic,
		coreprovider.ProviderOpenAI,
		coreprovider.ProviderGoogle,
		coreprovider.ProviderMoonshot,
		coreprovider.ProviderAlibaba,
	}
	displayNames := map[coreprovider.Provider]string{
		coreprovider.ProviderAnthropic: "Anthropic",
		coreprovider.ProviderOpenAI:    "OpenAI",
		coreprovider.ProviderGoogle:    "Google",
		coreprovider.ProviderMoonshot:  "Moonshot",
		coreprovider.ProviderAlibaba:   "Alibaba",
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
	s.tab = TabLLM
	s.width = width
	s.height = height

	// Load search providers
	s.loadSearchProviders()

	return nil
}

// loadSearchProviders loads the search provider list.
func (s *Model) loadSearchProviders() {
	currentProvider := ""
	if s.store != nil {
		currentProvider = s.store.GetSearchProvider()
	}
	if currentProvider == "" {
		currentProvider = string(search.ProviderExa)
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

// EnterModelSelect enters model selection mode.
func (s *Model) EnterModelSelect(ctx context.Context, width, height int) error {
	store, err := coreprovider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}
	s.store = store
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
	s.providers = nil
	s.searchProviders = nil

	current := store.GetCurrentModel()
	var currentModelID string
	if current != nil {
		currentModelID = current.ModelID
	}

	s.models = []ModelItem{}

	allModels := store.GetAllCachedModels()
	if len(allModels) == 0 {
		s.appendConnectedProviderModels(ctx, store, currentModelID)
	} else {
		s.appendCachedModels(allModels, currentModelID)
	}

	sortModelsWithCurrentFirst(s.models)

	s.filteredModels = s.models
	s.ensureVisible()
	s.active = true
	s.selectorType = SelectorTypeModel
	s.width = width
	s.height = height

	return nil
}

func (s *Model) appendConnectedProviderModels(ctx context.Context, store *coreprovider.Store, currentModelID string) {
	connections := store.GetConnections()
	for providerName, conn := range connections {
		p, err := coreprovider.GetProvider(ctx, coreprovider.Provider(providerName), conn.AuthMethod)
		if err != nil {
			continue
		}

		models, err := p.ListModels(ctx)
		if err != nil {
			continue
		}

		prov := coreprovider.Provider(providerName)
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
}

func (s *Model) appendCachedModels(allModels map[string][]coreprovider.ModelInfo, currentModelID string) {
	for key, models := range allModels {
		parts := strings.SplitN(key, ":", 2)
		providerName := key
		var authMethod coreprovider.AuthMethod
		if len(parts) >= 2 {
			providerName = parts[0]
			authMethod = coreprovider.AuthMethod(parts[1])
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

func (s *Model) disconnectAuthMethod(item AuthMethodItem, authMethods []AuthMethodItem, authIdx int) tea.Cmd {
	store, _ := coreprovider.NewStore()
	if store != nil {
		_ = store.Disconnect(item.Provider)
	}
	s.lastConnectResult = "✓ Disconnected"
	s.lastConnectAuthIdx = authIdx
	s.lastConnectSuccess = true
	authMethods[authIdx].Status = coreprovider.StatusAvailable
	return nil
}

func (s *Model) beginAuthConnection(authIdx int) {
	s.lastConnectResult = "Connecting..."
	s.lastConnectAuthIdx = authIdx
	s.lastConnectSuccess = false
}

func (s *Model) connectAuthMethod(item AuthMethodItem, authIdx int) tea.Cmd {
	s.beginAuthConnection(authIdx)
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

// sortModelsWithCurrentFirst sorts models with current model first, then by provider name.
func sortModelsWithCurrentFirst(models []ModelItem) {
	sort.SliceStable(models, func(i, j int) bool {
		if models[i].IsCurrent && !models[j].IsCurrent {
			return true
		}
		if !models[i].IsCurrent && models[j].IsCurrent {
			return false
		}
		return models[i].ProviderName < models[j].ProviderName
	})
}

// ConnectProvider connects to the selected provider and verifies the connection.
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

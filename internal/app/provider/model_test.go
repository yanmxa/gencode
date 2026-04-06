package provider

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
	"github.com/yanmxa/gencode/internal/ui/shared"
)

func TestCancelClearsTransientState(t *testing.T) {
	m := New()
	m.active = true
	m.selectorType = SelectorTypeModel
	m.level = LevelAuthMethod
	m.providers = []ProviderItem{{DisplayName: "OpenAI"}}
	m.models = []ModelItem{{ID: "gpt-5"}}
	m.searchProviders = []SearchProviderItem{{DisplayName: "Exa"}}
	m.selectedIdx = 2
	m.parentIdx = 1
	m.tab = TabSearch
	m.searchQuery = "gpt"
	m.filteredModels = []ModelItem{{ID: "gpt-5"}}
	m.scrollOffset = 3
	m.lastConnectResult = "Connected"
	m.lastConnectAuthIdx = 2
	m.lastConnectSuccess = true

	m.Cancel()

	if m.active {
		t.Fatal("Cancel should deactivate selector")
	}
	if m.selectorType != SelectorTypeProvider {
		t.Fatalf("selectorType = %v, want provider", m.selectorType)
	}
	if len(m.providers) != 0 || len(m.models) != 0 || len(m.searchProviders) != 0 {
		t.Fatal("Cancel should clear loaded selector data")
	}
	if m.level != LevelProvider || m.selectedIdx != 0 || m.parentIdx != 0 {
		t.Fatal("Cancel should reset navigation state")
	}
	if m.tab != TabLLM {
		t.Fatalf("tab = %v, want TabLLM", m.tab)
	}
	if m.searchQuery != "" || m.filteredModels != nil || m.scrollOffset != 0 {
		t.Fatal("Cancel should clear model search state")
	}
	if m.lastConnectResult != "" || m.lastConnectAuthIdx != 0 || m.lastConnectSuccess {
		t.Fatal("Cancel should clear connection result state")
	}
}

func TestGoBackResetsInlineConnectState(t *testing.T) {
	m := New()
	m.level = LevelAuthMethod
	m.parentIdx = 2
	m.lastConnectResult = "Connected"
	m.lastConnectAuthIdx = 1
	m.lastConnectSuccess = true

	if !m.GoBack() {
		t.Fatal("GoBack should return true from auth level")
	}
	if m.level != LevelProvider || m.selectedIdx != 2 {
		t.Fatal("GoBack should return to provider level and restore provider selection")
	}
	if m.lastConnectResult != "" || m.lastConnectAuthIdx != 0 || m.lastConnectSuccess {
		t.Fatal("GoBack should clear inline connect state")
	}
}

func TestHandleKeypressTabSwitchClearsInlineResult(t *testing.T) {
	m := New()
	m.selectorType = SelectorTypeProvider
	m.level = LevelProvider
	m.tab = TabLLM
	m.selectedIdx = 3
	m.lastConnectResult = "Connected"
	m.lastConnectAuthIdx = 2
	m.lastConnectSuccess = true

	m.HandleKeypress(tea.KeyMsg{Type: tea.KeyTab})

	if m.tab != TabSearch {
		t.Fatalf("tab = %v, want TabSearch", m.tab)
	}
	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d, want 0", m.selectedIdx)
	}
	if m.lastConnectResult != "" || m.lastConnectAuthIdx != 0 || m.lastConnectSuccess {
		t.Fatal("tab switch should clear inline connect result")
	}
}

func TestHandleKeypressEscClearsModelSearchBeforeDismiss(t *testing.T) {
	m := New()
	m.active = true
	m.selectorType = SelectorTypeModel
	m.models = []ModelItem{
		{ID: "gpt-5", DisplayName: "GPT-5", ProviderName: "openai"},
		{ID: "claude", DisplayName: "Claude", ProviderName: "anthropic"},
	}
	m.filteredModels = m.models[:1]
	m.searchQuery = "gpt"

	cmd := m.HandleKeypress(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatal("expected first Esc with active search to only clear search")
	}
	if m.searchQuery != "" {
		t.Fatalf("searchQuery = %q, want empty", m.searchQuery)
	}
	if len(m.filteredModels) != len(m.models) {
		t.Fatal("clearing search should restore full model list")
	}
	if !m.active {
		t.Fatal("clearing search should not dismiss selector")
	}
}

func TestHandleKeypressEscDismissesAfterSearchCleared(t *testing.T) {
	m := New()
	m.active = true

	cmd := m.HandleKeypress(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected dismiss command on Esc")
	}
	msg := cmd()
	if _, ok := msg.(shared.DismissedMsg); !ok {
		t.Fatalf("dismiss command returned %T, want shared.DismissedMsg", msg)
	}
	if m.active {
		t.Fatal("dismiss should deactivate selector")
	}
}

func TestSelectModelReturnsSelectionMessage(t *testing.T) {
	m := New()
	m.active = true
	m.selectorType = SelectorTypeModel
	m.filteredModels = []ModelItem{{
		ID:           "gpt-5",
		ProviderName: "openai",
		AuthMethod:   coreprovider.AuthAPIKey,
	}}

	cmd := m.Select()
	if cmd == nil {
		t.Fatal("Select should return command for selected model")
	}
	msg := cmd()
	selected, ok := msg.(ModelSelectedMsg)
	if !ok {
		t.Fatalf("selection returned %T, want ModelSelectedMsg", msg)
	}
	if selected.ModelID != "gpt-5" || selected.ProviderName != "openai" || selected.AuthMethod != coreprovider.AuthAPIKey {
		t.Fatalf("unexpected selection: %+v", selected)
	}
	if m.active {
		t.Fatal("model selection should close selector")
	}
}

func newTestStore(t *testing.T) *coreprovider.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := coreprovider.NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

func TestEnterModelSelect_UsesCachedModelsAndPutsCurrentFirst(t *testing.T) {
	store := newTestStore(t)
	if err := store.CacheModels(coreprovider.ProviderOpenAI, coreprovider.AuthAPIKey, []coreprovider.ModelInfo{
		{ID: "gpt-5-mini", DisplayName: "GPT-5 mini", InputTokenLimit: 128000, OutputTokenLimit: 16000},
		{ID: "gpt-5", DisplayName: "GPT-5", InputTokenLimit: 256000, OutputTokenLimit: 32000},
	}); err != nil {
		t.Fatalf("CacheModels() error = %v", err)
	}
	if err := store.SetCurrentModel("gpt-5", coreprovider.ProviderOpenAI, coreprovider.AuthAPIKey); err != nil {
		t.Fatalf("SetCurrentModel() error = %v", err)
	}

	m := New()
	if err := m.EnterModelSelect(context.Background(), 80, 24); err != nil {
		t.Fatalf("EnterModelSelect() error = %v", err)
	}

	if !m.active || m.selectorType != SelectorTypeModel {
		t.Fatalf("expected active model selector, got active=%v type=%v", m.active, m.selectorType)
	}
	if len(m.models) != 2 || len(m.filteredModels) != 2 {
		t.Fatalf("expected 2 models, got models=%d filtered=%d", len(m.models), len(m.filteredModels))
	}
	if m.models[0].ID != "gpt-5" || !m.models[0].IsCurrent {
		t.Fatalf("expected current model first, got %#v", m.models[0])
	}
	if m.models[0].InputTokenLimit != 256000 || m.models[0].OutputTokenLimit != 32000 {
		t.Fatalf("expected token limits copied to model item, got %#v", m.models[0])
	}
}

func TestUpdateFilterMatchesModelIDDisplayNameAndProvider(t *testing.T) {
	m := New()
	m.selectorType = SelectorTypeModel
	m.models = []ModelItem{
		{ID: "gpt-5", DisplayName: "GPT-5", ProviderName: "openai"},
		{ID: "claude-sonnet", DisplayName: "Claude Sonnet", ProviderName: "anthropic"},
	}

	m.searchQuery = "g5"
	m.updateFilter()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ID != "gpt-5" {
		t.Fatalf("expected ID fuzzy match to find gpt-5, got %#v", m.filteredModels)
	}

	m.searchQuery = "clsn"
	m.updateFilter()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ID != "claude-sonnet" {
		t.Fatalf("expected display-name fuzzy match to find claude-sonnet, got %#v", m.filteredModels)
	}

	m.searchQuery = "oa"
	m.updateFilter()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ProviderName != "openai" {
		t.Fatalf("expected provider-name fuzzy match to find openai model, got %#v", m.filteredModels)
	}
}

func TestLoadSearchProviders_DefaultsToExaAndPersistsSelection(t *testing.T) {
	store := newTestStore(t)
	m := New()
	m.store = store

	m.loadSearchProviders()
	if len(m.searchProviders) != 3 {
		t.Fatalf("expected 3 search providers, got %d", len(m.searchProviders))
	}
	if m.searchProviders[0].Name != search.ProviderExa || m.searchProviders[0].Status != "current" {
		t.Fatalf("expected Exa current by default, got %#v", m.searchProviders[0])
	}

	t.Setenv("BRAVE_API_KEY", "test-key")
	m.loadSearchProviders()
	m.selectedIdx = 2 // Brave
	cmd := m.selectSearchProvider()
	if cmd == nil {
		t.Fatal("expected selection command for available search provider")
	}
	msg := cmd()
	selected, ok := msg.(SearchProviderSelectedMsg)
	if !ok {
		t.Fatalf("selection returned %T, want SearchProviderSelectedMsg", msg)
	}
	if selected.Provider != search.ProviderBrave {
		t.Fatalf("selected provider = %q, want %q", selected.Provider, search.ProviderBrave)
	}
	if m.searchProviders[2].Status != "current" || m.searchProviders[0].Status != "available" {
		t.Fatalf("expected status transition from exa->brave, got %#v", m.searchProviders)
	}
	if got := store.GetSearchProvider(); got != string(search.ProviderBrave) {
		t.Fatalf("stored search provider = %q, want %q", got, search.ProviderBrave)
	}
	if m.lastConnectResult != "\u2713 Selected" || !m.lastConnectSuccess {
		t.Fatalf("unexpected selection status: result=%q success=%v", m.lastConnectResult, m.lastConnectSuccess)
	}
}

func TestSelectSearchProvider_ShowsMissingEnvForUnavailableProvider(t *testing.T) {
	store := newTestStore(t)
	t.Setenv("SERPER_API_KEY", "")

	m := New()
	m.store = store
	m.loadSearchProviders()
	m.selectedIdx = 1 // Serper

	cmd := m.selectSearchProvider()
	if cmd != nil {
		t.Fatal("expected no command when provider is unavailable")
	}
	if m.lastConnectSuccess {
		t.Fatal("expected missing-key selection to be marked unsuccessful")
	}
	if m.lastConnectResult != "Missing: SERPER_API_KEY" {
		t.Fatalf("unexpected missing-key message: %q", m.lastConnectResult)
	}
	if got := store.GetSearchProvider(); got != "" {
		t.Fatalf("expected store unchanged after failed selection, got %q", got)
	}
}

func TestSetModelPersistsSelection(t *testing.T) {
	store := newTestStore(t)
	m := New()
	m.store = store

	result, err := m.SetModel("gpt-5", "openai", coreprovider.AuthAPIKey)
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if result != "Model set to: gpt-5 (openai)" {
		t.Fatalf("unexpected result: %q", result)
	}

	current := store.GetCurrentModel()
	if current == nil || current.ModelID != "gpt-5" || current.Provider != coreprovider.ProviderOpenAI || current.AuthMethod != coreprovider.AuthAPIKey {
		t.Fatalf("unexpected current model after SetModel: %#v", current)
	}
}

package providerui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/app/kit"
)

func TestCancelClearsTransientState(t *testing.T) {
	m := New()
	m.active = true
	m.connectedProviders = []providerItem{{DisplayName: "Anthropic"}}
	m.allProviders = []providerItem{{DisplayName: "Google"}}
	m.allModels = []modelItem{{ID: "gpt-5"}}
	m.filteredModels = []modelItem{{ID: "gpt-5"}}
	m.visibleItems = []listItem{{Kind: itemModel}}
	m.expandedProviderIdx = 1
	m.apiKeyActive = true
	m.selectedIdx = 2
	m.scrollOffset = 3
	m.searchQuery = "gpt"
	m.lastConnectResult = "Connected"
	m.lastConnectAuthIdx = 2
	m.lastConnectSuccess = true

	m.Cancel()

	if m.active {
		t.Fatal("Cancel should deactivate selector")
	}
	if len(m.connectedProviders) != 0 || len(m.allProviders) != 0 {
		t.Fatal("Cancel should clear provider lists")
	}
	if len(m.allModels) != 0 || len(m.filteredModels) != 0 || len(m.visibleItems) != 0 {
		t.Fatal("Cancel should clear model/item lists")
	}
	if m.expandedProviderIdx != -1 || m.apiKeyActive {
		t.Fatal("Cancel should reset expansion and API key state")
	}
	if m.selectedIdx != 0 || m.scrollOffset != 0 {
		t.Fatal("Cancel should reset navigation state")
	}
	if m.searchQuery != "" {
		t.Fatal("Cancel should clear search query")
	}
	if m.lastConnectResult != "" || m.lastConnectAuthIdx != 0 || m.lastConnectSuccess {
		t.Fatal("Cancel should clear connection result state")
	}
}

func TestGoBackCollapsesAuthMethods(t *testing.T) {
	m := New()
	m.expandedProviderIdx = 2
	m.lastConnectResult = "Connected"
	m.lastConnectAuthIdx = 1
	m.lastConnectSuccess = true

	// Seed minimal data for rebuild
	m.activeTab = tabProviders
	m.allProviders = []providerItem{
		{DisplayName: "A"}, {DisplayName: "B"},
		{DisplayName: "C", AuthMethods: []authMethodItem{{DisplayName: "API"}}},
	}

	if !m.GoBack() {
		t.Fatal("GoBack should return true when auth methods are expanded")
	}
	if m.expandedProviderIdx != -1 {
		t.Fatal("GoBack should collapse expanded auth methods")
	}
	if m.lastConnectResult != "" || m.lastConnectAuthIdx != 0 || m.lastConnectSuccess {
		t.Fatal("GoBack should clear inline connect state")
	}
}

func TestGoBackCancelsAPIKeyInput(t *testing.T) {
	m := New()
	m.apiKeyActive = true

	if !m.GoBack() {
		t.Fatal("GoBack should return true when API key input is active")
	}
	if m.apiKeyActive {
		t.Fatal("GoBack should cancel API key input")
	}
}

func TestHandleKeypressEscClearsModelSearchBeforeDismiss(t *testing.T) {
	m := New()
	m.active = true
	m.allModels = []modelItem{
		{ID: "gpt-5", DisplayName: "GPT-5", ProviderName: "openai"},
		{ID: "claude", DisplayName: "Claude", ProviderName: "anthropic"},
	}
	m.connectedProviders = []providerItem{
		{Provider: "openai", DisplayName: "OpenAI"},
		{Provider: "anthropic", DisplayName: "Anthropic"},
	}
	m.searchQuery = "gpt"
	m.rebuildVisibleItems()

	cmd := m.HandleKeypress(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatal("expected first Esc with active search to only clear search")
	}
	if m.searchQuery != "" {
		t.Fatalf("searchQuery = %q, want empty", m.searchQuery)
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
	if _, ok := msg.(kit.DismissedMsg); !ok {
		t.Fatalf("dismiss command returned %T, want kit.DismissedMsg", msg)
	}
	if m.active {
		t.Fatal("dismiss should deactivate selector")
	}
}

func TestSelectModelReturnsSelectionMessage(t *testing.T) {
	m := New()
	m.active = true
	model := modelItem{
		ID:           "gpt-5",
		ProviderName: "openai",
		AuthMethod:   coreprovider.AuthAPIKey,
	}
	m.visibleItems = []listItem{
		{Kind: itemModel, Model: &model},
	}
	m.selectedIdx = 0

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

func TestEnterLoadsCachedModelsAndPutsCurrentFirst(t *testing.T) {
	store := newTestStore(t)
	if err := store.CacheModels(coreprovider.OpenAI, coreprovider.AuthAPIKey, []coreprovider.ModelInfo{
		{ID: "gpt-5-mini", DisplayName: "GPT-5 mini", InputTokenLimit: 128000, OutputTokenLimit: 16000},
		{ID: "gpt-5", DisplayName: "GPT-5", InputTokenLimit: 256000, OutputTokenLimit: 32000},
	}); err != nil {
		t.Fatalf("CacheModels() error = %v", err)
	}
	if err := store.Connect(coreprovider.OpenAI, coreprovider.AuthAPIKey); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := store.SetCurrentModel("gpt-5", coreprovider.OpenAI, coreprovider.AuthAPIKey); err != nil {
		t.Fatalf("SetCurrentModel() error = %v", err)
	}

	m := New()
	if _, err := m.Enter(context.Background(), 80, 24); err != nil {
		t.Fatalf("Enter() error = %v", err)
	}

	if !m.active {
		t.Fatal("expected active selector")
	}
	if len(m.allModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(m.allModels))
	}

	// Check visible items contain model rows
	modelCount := 0
	for _, item := range m.visibleItems {
		if item.Kind == itemModel {
			modelCount++
		}
	}
	if modelCount != 2 {
		t.Fatalf("expected 2 model items in visible list, got %d", modelCount)
	}
}

func TestUpdateFilterMatchesModelIDDisplayNameAndProvider(t *testing.T) {
	m := New()
	m.allModels = []modelItem{
		{ID: "gpt-5", DisplayName: "GPT-5", ProviderName: "openai"},
		{ID: "claude-sonnet", DisplayName: "Claude Sonnet", ProviderName: "anthropic"},
	}
	m.connectedProviders = []providerItem{
		{Provider: "openai", DisplayName: "OpenAI"},
		{Provider: "anthropic", DisplayName: "Anthropic"},
	}

	m.searchQuery = "g5"
	m.rebuildVisibleItems()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ID != "gpt-5" {
		t.Fatalf("expected ID fuzzy match to find gpt-5, got %#v", m.filteredModels)
	}

	m.searchQuery = "clsn"
	m.rebuildVisibleItems()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ID != "claude-sonnet" {
		t.Fatalf("expected display-name fuzzy match to find claude-sonnet, got %#v", m.filteredModels)
	}

	m.searchQuery = "oa"
	m.rebuildVisibleItems()
	if len(m.filteredModels) != 1 || m.filteredModels[0].ProviderName != "openai" {
		t.Fatalf("expected provider-name fuzzy match to find openai model, got %#v", m.filteredModels)
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
	if current == nil || current.ModelID != "gpt-5" || current.Provider != coreprovider.OpenAI || current.AuthMethod != coreprovider.AuthAPIKey {
		t.Fatalf("unexpected current model after SetModel: %#v", current)
	}
}

func TestTabSwitchesBetweenTabs(t *testing.T) {
	m := New()
	m.active = true
	m.activeTab = tabModels
	m.allModels = []modelItem{
		{ID: "gpt-5", DisplayName: "GPT-5", ProviderName: "openai"},
	}
	m.connectedProviders = []providerItem{
		{Provider: "openai", DisplayName: "OpenAI"},
	}
	m.allProviders = []providerItem{
		{Provider: "openai", DisplayName: "OpenAI", Connected: true},
		{Provider: "google", DisplayName: "Google", AuthMethods: []authMethodItem{
			{DisplayName: "API Key", Status: coreprovider.StatusNotConfigured},
		}},
	}
	m.rebuildVisibleItems()

	// Press Tab to switch to Providers tab
	m.HandleKeypress(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabProviders {
		t.Fatal("Tab should switch to Providers tab")
	}

	// Should have provider items now
	found := false
	for _, item := range m.visibleItems {
		if item.Kind == itemProvider {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Providers tab should show provider items")
	}

	// Press Tab again to go back to Models
	m.HandleKeypress(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != tabModels {
		t.Fatal("Tab should switch back to Models tab")
	}
}

func TestNavigationSkipsProviderHeaders(t *testing.T) {
	m := New()
	m.active = true

	model1 := modelItem{ID: "m1", ProviderName: "openai"}
	model2 := modelItem{ID: "m2", ProviderName: "anthropic"}
	m.visibleItems = []listItem{
		{Kind: itemProviderHeader}, // 0 - not selectable
		{Kind: itemModel, Model: &model1}, // 1
		{Kind: itemProviderHeader}, // 2 - not selectable
		{Kind: itemModel, Model: &model2}, // 3
	}
	m.selectedIdx = 1

	// MoveDown should skip index 2 (header) and land on 3
	m.MoveDown()
	if m.selectedIdx != 3 {
		t.Fatalf("MoveDown should skip header, got selectedIdx=%d, want 3", m.selectedIdx)
	}

	// MoveUp should skip index 2 (header) and land on 1
	m.MoveUp()
	if m.selectedIdx != 1 {
		t.Fatalf("MoveUp should skip header, got selectedIdx=%d, want 1", m.selectedIdx)
	}
}

func TestSelectProviderExpandsAuthMethods(t *testing.T) {
	m := New()
	m.active = true
	m.activeTab = tabProviders
	m.allProviders = []providerItem{
		{
			Provider:    "anthropic",
			DisplayName: "Anthropic",
			AuthMethods: []authMethodItem{
				{DisplayName: "API Key", Status: coreprovider.StatusNotConfigured},
				{DisplayName: "Bedrock", Status: coreprovider.StatusAvailable},
			},
		},
	}
	m.rebuildVisibleItems()

	// Find the provider item
	for i, item := range m.visibleItems {
		if item.Kind == itemProvider {
			m.selectedIdx = i
			break
		}
	}

	// Select should expand auth methods (since there are multiple)
	cmd := m.Select()
	if cmd != nil {
		t.Fatal("selecting multi-auth provider should not return a command")
	}
	if m.expandedProviderIdx != 0 {
		t.Fatalf("expandedProviderIdx = %d, want 0", m.expandedProviderIdx)
	}

	// Check that auth method items are now in visible list
	authCount := 0
	for _, item := range m.visibleItems {
		if item.Kind == itemAuthMethod {
			authCount++
		}
	}
	if authCount != 2 {
		t.Fatalf("expected 2 auth method items, got %d", authCount)
	}
}

func TestRebuildVisibleItemsStructure(t *testing.T) {
	m := New()
	m.activeTab = tabModels
	m.allModels = []modelItem{
		{ID: "m1", ProviderName: "openai", DisplayName: "Model 1"},
		{ID: "m2", ProviderName: "openai", DisplayName: "Model 2"},
		{ID: "m3", ProviderName: "anthropic", DisplayName: "Model 3"},
	}
	m.connectedProviders = []providerItem{
		{Provider: "openai", DisplayName: "OpenAI"},
		{Provider: "anthropic", DisplayName: "Anthropic"},
	}

	m.rebuildVisibleItems()

	// Expected structure (Models tab):
	// 0: ProviderHeader (OpenAI)
	// 1: Model (m1)
	// 2: Model (m2)
	// 3: ProviderHeader (Anthropic)
	// 4: Model (m3)

	if len(m.visibleItems) != 5 {
		t.Fatalf("expected 5 visible items, got %d", len(m.visibleItems))
	}
	if m.visibleItems[0].Kind != itemProviderHeader {
		t.Fatalf("item 0 should be ProviderHeader, got %v", m.visibleItems[0].Kind)
	}
	if m.visibleItems[1].Kind != itemModel || m.visibleItems[2].Kind != itemModel {
		t.Fatal("items 1-2 should be Models")
	}
	if m.visibleItems[3].Kind != itemProviderHeader {
		t.Fatalf("item 3 should be ProviderHeader, got %v", m.visibleItems[3].Kind)
	}
	if m.visibleItems[4].Kind != itemModel {
		t.Fatal("item 4 should be Model")
	}

	// selectedIdx should skip the first header and land on index 1
	if m.selectedIdx != 1 {
		t.Fatalf("selectedIdx should be 1 (first model), got %d", m.selectedIdx)
	}
}

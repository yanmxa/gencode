package provider

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
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

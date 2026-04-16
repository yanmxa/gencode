package pluginui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/app/kit"
)

func TestCancelClearsTransientPluginSelectorState(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.active = true
	m.level = levelDetail
	m.selectedIdx = 3
	m.scrollOffset = 4
	m.parentIdx = 2
	m.searchQuery = "plug"
	m.filteredItems = []any{"plugin"}
	m.detailPlugin = &pluginItem{Name: "demo"}
	m.detailDiscover = &discoverPluginItem{Name: "discover"}
	m.detailMarketplace = &marketplaceItem{ID: "market"}
	m.actions = []action{{Label: "Back", Action: "back"}}
	m.actionIdx = 1
	m.addMarketplaceInput = "owner/repo"
	m.addDialogCursor = 1
	m.browseMarketplaceID = "market"
	m.browsePlugins = []discoverPluginItem{{Name: "child"}}
	m.isLoading = true
	m.loadingMsg = "loading"
	m.lastMessage = "oops"
	m.isError = true

	m.Cancel()

	if m.active {
		t.Fatal("Cancel should deactivate selector")
	}
	if m.level != levelTabList || m.selectedIdx != 0 || m.scrollOffset != 0 || m.parentIdx != 0 {
		t.Fatal("Cancel should reset list navigation state")
	}
	if m.searchQuery != "" || m.filteredItems != nil {
		t.Fatal("Cancel should clear search state")
	}
	if m.detailPlugin != nil || m.detailDiscover != nil || m.detailMarketplace != nil || m.actions != nil || m.actionIdx != 0 {
		t.Fatal("Cancel should clear detail state")
	}
	if m.addMarketplaceInput != "" || m.addDialogCursor != 0 {
		t.Fatal("Cancel should clear add-marketplace input state")
	}
	if m.browseMarketplaceID != "" || m.browsePlugins != nil {
		t.Fatal("Cancel should clear browse state")
	}
	if m.isLoading || m.loadingMsg != "" {
		t.Fatal("Cancel should clear loading state")
	}
	if m.lastMessage != "" || m.isError {
		t.Fatal("Cancel should clear status message")
	}
}

func TestHandleListEscClearsSearchBeforeDismiss(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.active = true
	m.activeTab = tabInstalled
	m.installedFlatList = []pluginItem{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m.searchQuery = "alp"
	m.filteredItems = []any{m.installedFlatList[0]}

	cmd := m.HandleKeypress(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Fatal("Esc with active search should only clear search")
	}
	if m.searchQuery != "" {
		t.Fatalf("searchQuery = %q, want empty", m.searchQuery)
	}
	if len(m.filteredItems) != len(m.installedFlatList) {
		t.Fatal("clearing search should restore full filtered list")
	}
	if !m.active {
		t.Fatal("clearing search should not dismiss selector")
	}
}

func TestHandleListEscDismissesSelector(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.active = true

	cmd := m.HandleKeypress(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc should return dismiss command when no search is active")
	}
	msg := cmd()
	if _, ok := msg.(kit.DismissedMsg); !ok {
		t.Fatalf("dismiss command returned %T, want kit.DismissedMsg", msg)
	}
	if m.active {
		t.Fatal("dismiss should deactivate selector")
	}
}

func TestSwitchTabResetsDetailStateAndSearch(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.activeTab = tabInstalled
	m.level = levelDetail
	m.selectedIdx = 3
	m.scrollOffset = 2
	m.parentIdx = 1
	m.searchQuery = "demo"
	m.detailPlugin = &pluginItem{Name: "demo"}
	m.actions = []action{{Label: "Back", Action: "back"}}
	m.marketplaces = []marketplaceItem{{ID: "market"}}

	m.switchTab(tabMarketplaces)

	if m.activeTab != tabMarketplaces {
		t.Fatalf("activeTab = %v, want tabMarketplaces", m.activeTab)
	}
	if m.level != levelTabList || m.selectedIdx != 0 || m.scrollOffset != 0 || m.parentIdx != 0 {
		t.Fatal("switchTab should reset list navigation")
	}
	if m.searchQuery != "" {
		t.Fatal("switchTab should clear search query")
	}
	if m.detailPlugin != nil || m.actions != nil || m.actionIdx != 0 {
		t.Fatal("switchTab should clear detail state")
	}
	if len(m.filteredItems) != len(m.marketplaces) {
		t.Fatal("switchTab should refresh filtered items for the active tab")
	}
}

func TestToggleSelectedPluginReturnsDisableMsg(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.activeTab = tabInstalled
	m.level = levelTabList
	m.filteredItems = []any{
		pluginItem{
			Name:     "demo",
			FullName: "demo",
			Enabled:  true,
			Scope:    coreplugin.ScopeUser,
		},
	}

	cmd := m.toggleSelectedPlugin()
	if cmd == nil {
		t.Fatal("toggleSelectedPlugin should return a command")
	}
	msg := cmd()
	if disable, ok := msg.(DisableMsg); !ok || disable.PluginName != "demo" {
		t.Fatalf("unexpected toggle message: %#v", msg)
	}
}

func TestRenderTabListShowsPluginManagerFrame(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.active = true
	m.width = 100
	m.height = 30
	m.activeTab = tabInstalled
	m.installedFlatList = []pluginItem{{Name: "demo", Description: "demo plugin"}}
	m.filteredItems = []any{m.installedFlatList[0]}

	rendered := m.Render()
	for _, want := range []string{"Discover", "Installed", "Marketplaces", "filter"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q in output:\n%s", want, rendered)
		}
	}
}

func TestRenderInstalledDetailShowsStructuredSections(t *testing.T) {
	m := New(coreplugin.NewRegistry())
	m.active = true
	m.width = 100
	m.height = 30
	m.level = levelDetail
	m.detailPlugin = &pluginItem{
		Name:        "deploy",
		FullName:    "deploy@corp",
		Description: "Deploy safely",
		Enabled:     true,
		Scope:       coreplugin.ScopeProject,
		Skills:      1,
		Agents:      2,
		Hooks:       1,
	}
	m.actions = []action{{Label: "Disable plugin", Action: "disable"}, {Label: "Back", Action: "back"}}

	rendered := m.Render()
	for _, want := range []string{"Plugin Details", "deploy@corp", "Status:", "Scope:", "Components", "Disable plugin"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q in output:\n%s", want, rendered)
		}
	}
}

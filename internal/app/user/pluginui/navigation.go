package pluginui

import (
	"strings"

	"github.com/yanmxa/gencode/internal/app/kit"
)

// Tab navigation
func (s *Model) NextTab() { s.switchTab((s.activeTab + 1) % 3) }
func (s *Model) PrevTab() { s.switchTab((s.activeTab + 2) % 3) }

func (s *Model) switchTab(tab Tab) {
	s.activeTab = tab
	s.resetListState()
	s.resetDetailState()
	s.resetBrowseState()
	s.searchQuery = ""
	s.refreshCurrentTab()
}

// updateFilter filters items based on search query
func (s *Model) updateFilter() {
	query := strings.ToLower(s.searchQuery)
	s.filteredItems = s.filterItemsForTab(query)
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// filterItemsForTab returns filtered items based on the active tab and query
func (s *Model) filterItemsForTab(query string) []any {
	switch s.activeTab {
	case tabInstalled:
		return filterItems(s.installedFlatList, query, func(p pluginItem) []string {
			return []string{p.Name, p.Description}
		})
	case tabDiscover:
		return filterItems(s.discoverPlugins, query, func(p discoverPluginItem) []string {
			return []string{p.Name, p.Description, p.Marketplace}
		})
	case tabMarketplaces:
		return filterItems(s.marketplaces, query, func(m marketplaceItem) []string {
			return []string{m.ID, m.Source}
		})
	default:
		return nil
	}
}

// filterItems is a generic filter function for any slice type
func filterItems[T any](items []T, query string, getFields func(T) []string) []any {
	if query == "" {
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result
	}

	result := make([]any, 0, len(items))
	for _, item := range items {
		for _, field := range getFields(item) {
			if kit.FuzzyMatch(strings.ToLower(field), query) {
				result = append(result, item)
				break
			}
		}
	}
	return result
}

// Navigation
func (s *Model) MoveUp() {
	s.clearMessage()
	switch s.level {
	case levelDetail, levelInstallOptions:
		if s.actionIdx > 0 {
			s.actionIdx--
		} else if s.detailScroll > 0 {
			s.detailScroll--
		}
	default:
		if s.selectedIdx > 0 {
			s.selectedIdx--
			s.ensureVisible()
		}
	}
}

func (s *Model) MoveDown() {
	s.clearMessage()
	switch s.level {
	case levelDetail, levelInstallOptions:
		if s.actionIdx < len(s.actions)-1 {
			s.actionIdx++
		} else {
			s.detailScroll++
		}
	default:
		maxIdx := s.getMaxIndex()
		if s.selectedIdx < maxIdx {
			s.selectedIdx++
			s.ensureVisible()
		}
	}
}

// getMaxIndex returns the maximum selectable index for the current view.
func (s *Model) getMaxIndex() int {
	switch s.level {
	case levelBrowsePlugins:
		return len(s.browsePlugins) - 1
	default:
		maxIdx := len(s.filteredItems) - 1
		if s.activeTab == tabMarketplaces {
			maxIdx++
		}
		return maxIdx
	}
}

func (s *Model) ensureVisible() {
	visible := s.maxVisible
	switch s.level {
	case levelBrowsePlugins:
		visible = max(4, s.height-14)
	default:
		switch s.activeTab {
		case tabDiscover:
			visible = max(3, (s.height-14)/3)
		case tabMarketplaces:
			visible = max(4, (s.height-14)/2)
		default:
			visible = max(4, s.height-14)
		}
	}
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+visible {
		s.scrollOffset = s.selectedIdx - visible + 1
	}
}

// enterDetail enters the detail view for the selected item.
func (s *Model) enterDetail() {
	s.parentIdx = s.selectedIdx

	switch s.activeTab {
	case tabInstalled:
		s.enterInstalledDetail()
	case tabDiscover:
		s.enterDiscoverDetail()
	case tabMarketplaces:
		s.enterMarketplaceDetail()
	}
}

func (s *Model) enterInstalledDetail() {
	if s.selectedIdx >= len(s.filteredItems) {
		return
	}
	if p, ok := s.filteredItems[s.selectedIdx].(pluginItem); ok {
		s.detailPlugin = &p
		s.actions = s.buildInstalledActions(p)
		s.actionIdx = 0
		s.level = levelDetail
	}
}

func (s *Model) enterDiscoverDetail() {
	if s.selectedIdx >= len(s.filteredItems) {
		return
	}
	if p, ok := s.filteredItems[s.selectedIdx].(discoverPluginItem); ok {
		s.detailDiscover = &p
		s.actions = s.buildDiscoverActions(p)
		s.actionIdx = 0
		s.level = levelDetail
	}
}

func (s *Model) enterMarketplaceDetail() {
	if s.selectedIdx == 0 {
		s.level = levelAddMarketplace
		s.addMarketplaceInput = ""
		s.addDialogCursor = 0
		return
	}
	mktIdx := s.selectedIdx - 1
	if mktIdx >= len(s.filteredItems) {
		return
	}
	if m, ok := s.filteredItems[mktIdx].(marketplaceItem); ok {
		s.detailMarketplace = &m
		s.actions = s.buildMarketplaceActions(m)
		s.actionIdx = 0
		s.level = levelDetail
	}
}

// goBack returns to the previous view.
func (s *Model) goBack() bool {
	switch s.level {
	case levelDetail:
		s.level = levelTabList
		s.selectedIdx = s.parentIdx
		s.resetDetailState()
		s.clearMessage()
		return true
	case levelInstallOptions:
		s.level = levelDetail
		s.actions = s.buildDiscoverActions(*s.detailDiscover)
		s.actionIdx = 0
		return true
	case levelAddMarketplace:
		s.level = levelTabList
		s.addMarketplaceInput = ""
		return true
	case levelBrowsePlugins:
		s.level = levelDetail
		s.resetBrowseState()
		s.selectedIdx = 0
		return true
	}
	return false
}

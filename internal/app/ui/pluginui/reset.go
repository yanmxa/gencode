package pluginui

func (s *Model) resetListState() {
	s.level = levelTabList
	s.selectedIdx = 0
	s.scrollOffset = 0
	s.parentIdx = 0
}

func (s *Model) resetDetailState() {
	s.detailPlugin = nil
	s.detailDiscover = nil
	s.detailMarketplace = nil
	s.actions = nil
	s.actionIdx = 0
	s.detailScroll = 0
}

func (s *Model) resetBrowseState() {
	s.browseMarketplaceID = ""
	s.browsePlugins = nil
}

func (s *Model) resetInputState() {
	s.searchQuery = ""
	s.filteredItems = nil
	s.addMarketplaceInput = ""
	s.addDialogCursor = 0
}

func (s *Model) resetLoadingState() {
	s.isLoading = false
	s.loadingMsg = ""
}

// Cancel cancels the selector and clears transient UI state.
func (s *Model) Cancel() {
	s.active = false
	s.resetListState()
	s.resetDetailState()
	s.resetBrowseState()
	s.resetInputState()
	s.resetLoadingState()
	s.clearMessage()
}

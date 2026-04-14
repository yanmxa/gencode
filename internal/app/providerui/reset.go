package providerui

func (s *Model) resetConnectionResult() {
	s.lastConnectResult = ""
	s.lastConnectAuthIdx = 0
	s.lastConnectSuccess = false
}

func (s *Model) resetModelSearch() {
	s.searchQuery = ""
	s.filteredModels = nil
	s.scrollOffset = 0
}

func (s *Model) resetNavigation() {
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// Cancel cancels the selector and clears transient state so the next open starts cleanly.
func (s *Model) Cancel() {
	s.active = false
	s.connectedProviders = nil
	s.allProviders = nil
	s.allModels = nil
	s.filteredModels = nil
	s.visibleItems = nil
	s.expandedProviderIdx = -1
	s.apiKeyActive = false
	s.store = nil
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
}

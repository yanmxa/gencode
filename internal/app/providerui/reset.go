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
	s.parentIdx = 0
	s.level = LevelProvider
}

// Cancel cancels the selector and clears transient state so the next open starts cleanly.
func (s *Model) Cancel() {
	s.active = false
	s.selectorType = SelectorTypeProvider
	s.providers = nil
	s.models = nil
	s.searchProviders = nil
	s.tab = TabLLM
	s.store = nil
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
}

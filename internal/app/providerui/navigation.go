package providerui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/ui/selector"
)

// ensureVisible adjusts scrollOffset to keep selectedIdx visible.
func (s *Model) ensureVisible() {
	if s.selectorType != SelectorTypeModel {
		return
	}
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

// updateFilter filters models based on search query (fuzzy match).
func (s *Model) updateFilter() {
	if s.searchQuery == "" {
		s.filteredModels = s.models
	} else {
		query := strings.ToLower(s.searchQuery)
		s.filteredModels = make([]ModelItem, 0)
		for _, m := range s.models {
			if selector.FuzzyMatch(strings.ToLower(m.ID), query) ||
				selector.FuzzyMatch(strings.ToLower(m.DisplayName), query) ||
				selector.FuzzyMatch(strings.ToLower(m.ProviderName), query) {
				s.filteredModels = append(s.filteredModels, m)
			}
		}
	}
	s.selectedIdx = 0
	s.scrollOffset = 0
}

// MoveUp moves the selection up.
func (s *Model) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
		s.ensureVisible()
	}
}

// MoveDown moves the selection down.
func (s *Model) MoveDown() {
	maxIdx := s.getMaxIndex()
	if s.selectedIdx < maxIdx {
		s.selectedIdx++
		s.ensureVisible()
	}
}

// getMaxIndex returns the maximum selectable index for current level.
func (s *Model) getMaxIndex() int {
	if s.selectorType == SelectorTypeModel {
		return len(s.filteredModels) - 1
	}
	if s.tab == TabSearch {
		return len(s.searchProviders) - 1
	}
	if s.level == LevelProvider {
		return len(s.providers) - 1
	}
	if s.parentIdx < len(s.providers) {
		return len(s.providers[s.parentIdx].AuthMethods) - 1
	}
	return 0
}

// GoBack goes back to the previous level.
func (s *Model) GoBack() bool {
	if s.level != LevelAuthMethod {
		return false
	}
	s.level = LevelProvider
	s.selectedIdx = s.parentIdx
	s.resetConnectionResult()
	return true
}

func (s *Model) switchProviderTab() {
	if s.tab == TabLLM {
		s.tab = TabSearch
	} else {
		s.tab = TabLLM
	}
	s.selectedIdx = 0
	s.resetConnectionResult()
}

func (s *Model) clearModelSearch() bool {
	if s.selectorType != SelectorTypeModel || s.searchQuery == "" {
		return false
	}
	s.searchQuery = ""
	s.updateFilter()
	return true
}

func (s *Model) trimModelSearch() {
	if s.selectorType != SelectorTypeModel || len(s.searchQuery) == 0 {
		return
	}
	s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
	s.updateFilter()
}

func (s *Model) appendModelSearch(text string) bool {
	if s.selectorType != SelectorTypeModel {
		return false
	}
	s.searchQuery += text
	s.updateFilter()
	return true
}

func (s *Model) allowsVimNavigation() bool {
	return s.selectorType != SelectorTypeModel || s.searchQuery == ""
}

// HandleKeypress handles a keypress and returns a command if selection is made.
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyTab:
		if s.selectorType == SelectorTypeProvider && s.level == LevelProvider {
			s.switchProviderTab()
		}
		return nil
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil
	case tea.KeyEnter, tea.KeyRight:
		return s.Select()
	case tea.KeyLeft:
		s.GoBack()
		return nil
	case tea.KeyEsc:
		if s.clearModelSearch() {
			return nil
		}
		if s.GoBack() {
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return selector.DismissedMsg{} }
	case tea.KeyBackspace:
		s.trimModelSearch()
		return nil
	case tea.KeySpace:
		if s.appendModelSearch(" ") {
			return nil
		}
	case tea.KeyRunes:
		if s.appendModelSearch(string(key.Runes)) {
			return nil
		}
	}

	if !s.allowsVimNavigation() {
		return nil
	}

	switch key.String() {
	case "j":
		s.MoveDown()
	case "k":
		s.MoveUp()
	case "l":
		return s.Select()
	case "h":
		s.GoBack()
	}

	return nil
}

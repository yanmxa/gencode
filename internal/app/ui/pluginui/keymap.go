package pluginui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/ui/selector"
)

// HandleKeypress handles a keypress and returns a command if needed.
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	if s.level == levelAddMarketplace {
		return s.handleAddMarketplaceKeypress(key)
	}
	if s.level == levelDetail || s.level == levelInstallOptions {
		return s.handleDetailKeypress(key)
	}
	if s.level == levelBrowsePlugins {
		return s.handleBrowseKeypress(key)
	}
	return s.handleListKeypress(key)
}

func (s *Model) handleAddMarketplaceKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEsc:
		s.goBack()
		return nil
	case tea.KeyEnter:
		return s.addMarketplace()
	case tea.KeyBackspace:
		if len(s.addMarketplaceInput) > 0 {
			s.addMarketplaceInput = s.addMarketplaceInput[:len(s.addMarketplaceInput)-1]
		}
		return nil
	case tea.KeyRunes:
		input := key.String()
		if s.addMarketplaceInput == "" {
			input = strings.TrimPrefix(input, "[")
		}
		input = strings.TrimSuffix(input, "]")
		if input != "" {
			s.addMarketplaceInput += input
		}
		return nil
	}
	return nil
}

func (s *Model) handleDetailKeypress(key tea.KeyMsg) tea.Cmd {
	if s.handleNavigationKey(key, true) {
		return nil
	}
	switch key.Type {
	case tea.KeyEnter:
		return s.executeAction()
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
	case tea.KeyRunes:
		if key.String() == "h" {
			s.goBack()
		}
	}
	return nil
}

func (s *Model) handleBrowseKeypress(key tea.KeyMsg) tea.Cmd {
	if s.handleNavigationKey(key, true) {
		return nil
	}
	switch key.Type {
	case tea.KeyEnter:
		if s.selectedIdx < len(s.browsePlugins) {
			p := s.browsePlugins[s.selectedIdx]
			s.detailDiscover = &p
			s.actions = s.buildDiscoverActions(p)
			s.actionIdx = 0
			s.level = levelDetail
		}
	case tea.KeyEsc, tea.KeyLeft:
		s.goBack()
	}
	return nil
}

// handleNavigationKey handles common up/down navigation keys, returns true if handled.
func (s *Model) handleNavigationKey(key tea.KeyMsg, vimKeys bool) bool {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return true
	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return true
	case tea.KeyRunes:
		if vimKeys {
			switch key.String() {
			case "k":
				s.MoveUp()
				return true
			case "j":
				s.MoveDown()
				return true
			}
		}
	}
	return false
}

func (s *Model) handleListKeypress(key tea.KeyMsg) tea.Cmd {
	if s.searchQuery == "" {
		switch key.Type {
		case tea.KeyTab, tea.KeyRight:
			s.NextTab()
			return nil
		case tea.KeyShiftTab, tea.KeyLeft:
			s.PrevTab()
			return nil
		}
	}

	if s.handleNavigationKey(key, s.searchQuery == "") {
		return nil
	}

	switch key.Type {
	case tea.KeyEnter:
		s.enterDetail()
		return nil
	case tea.KeyEsc:
		if s.searchQuery != "" {
			s.searchQuery = ""
			s.updateFilter()
			return nil
		}
		s.Cancel()
		return func() tea.Msg { return selector.DismissedMsg{} }
	case tea.KeyBackspace:
		if len(s.searchQuery) > 0 {
			s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
			s.updateFilter()
		}
		return nil
	case tea.KeyRunes:
		return s.handleListRuneKey(key.String())
	}
	return nil
}

// handleListRuneKey handles rune key input in list view.
func (s *Model) handleListRuneKey(r string) tea.Cmd {
	if s.searchQuery == "" {
		switch r {
		case "l":
			s.enterDetail()
			return nil
		case " ":
			return s.toggleSelectedPlugin()
		case "u":
			return s.handleMarketplaceAction(func(m marketplaceItem) tea.Cmd {
				return s.syncMarketplace(m.ID)
			})
		case "r":
			return s.handleMarketplaceAction(func(m marketplaceItem) tea.Cmd {
				return func() tea.Msg { return MarketplaceRemoveMsg{ID: m.ID} }
			})
		}
	}
	s.searchQuery += r
	s.updateFilter()
	return nil
}

// handleMarketplaceAction executes an action on the selected marketplace.
func (s *Model) handleMarketplaceAction(action func(marketplaceItem) tea.Cmd) tea.Cmd {
	if s.activeTab != tabMarketplaces || s.selectedIdx == 0 {
		return nil
	}
	mktIdx := s.selectedIdx - 1
	if mktIdx < len(s.filteredItems) {
		if m, ok := s.filteredItems[mktIdx].(marketplaceItem); ok {
			return action(m)
		}
	}
	return nil
}

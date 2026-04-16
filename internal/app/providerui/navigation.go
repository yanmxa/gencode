package providerui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/selector"
)

func (s *Model) ensureVisible() {
	if s.selectedIdx < s.scrollOffset {
		s.scrollOffset = s.selectedIdx
	}
	if s.selectedIdx >= s.scrollOffset+s.maxVisible {
		s.scrollOffset = s.selectedIdx - s.maxVisible + 1
	}
}

func (s *Model) MoveUp() {
	for s.selectedIdx > 0 {
		s.selectedIdx--
		if s.visibleItems[s.selectedIdx].Kind != itemProviderHeader {
			break
		}
	}
	s.ensureVisible()
}

func (s *Model) MoveDown() {
	for s.selectedIdx < len(s.visibleItems)-1 {
		s.selectedIdx++
		if s.visibleItems[s.selectedIdx].Kind != itemProviderHeader {
			break
		}
	}
	s.ensureVisible()
}

func (s *Model) switchTab(t tab) {
	if t == s.activeTab {
		return
	}
	s.activeTab = t
	s.resetNavigation()
	s.resetModelSearch()
	s.resetConnectionResult()
	s.expandedProviderIdx = -1
	s.apiKeyActive = false
	s.rebuildVisibleItems()
}

func (s *Model) NextTab() { s.switchTab((s.activeTab + 1) % 2) }
func (s *Model) PrevTab() { s.switchTab((s.activeTab + 1 + 2) % 2) }

func (s *Model) GoBack() bool {
	if s.apiKeyActive {
		s.apiKeyActive = false
		return true
	}
	if s.expandedProviderIdx >= 0 {
		s.expandedProviderIdx = -1
		s.resetConnectionResult()
		s.rebuildVisibleItems()
		return true
	}
	return false
}

func (s *Model) clearModelSearch() bool {
	if s.searchQuery == "" {
		return false
	}
	s.searchQuery = ""
	s.rebuildVisibleItems()
	return true
}

func (s *Model) trimModelSearch() {
	if len(s.searchQuery) == 0 {
		return
	}
	s.searchQuery = s.searchQuery[:len(s.searchQuery)-1]
	s.rebuildVisibleItems()
}

func (s *Model) appendModelSearch(text string) {
	s.searchQuery += text
	s.rebuildVisibleItems()
}

func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	// Route to API key input if active
	if s.apiKeyActive {
		return s.handleAPIKeyInput(key)
	}

	switch key.Type {
	case tea.KeyTab:
		if s.searchQuery == "" {
			s.NextTab()
		}
		return nil

	case tea.KeyShiftTab:
		if s.searchQuery == "" {
			s.PrevTab()
		}
		return nil

	case tea.KeyUp, tea.KeyCtrlP:
		s.MoveUp()
		return nil

	case tea.KeyDown, tea.KeyCtrlN:
		s.MoveDown()
		return nil

	case tea.KeyEnter:
		return s.Select()

	case tea.KeyRight:
		if s.searchQuery == "" {
			s.NextTab()
		}
		return nil

	case tea.KeyLeft:
		if s.searchQuery == "" && !s.GoBack() {
			s.PrevTab()
		}
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
		s.appendModelSearch(" ")
		return nil

	case tea.KeyRunes:
		s.appendModelSearch(string(key.Runes))
		return nil
	}

	// Vim navigation (only when search query is empty)
	if s.searchQuery == "" {
		switch key.String() {
		case "j":
			s.MoveDown()
		case "k":
			s.MoveUp()
		case "l":
			s.NextTab()
		case "h":
			if !s.GoBack() {
				s.PrevTab()
			}
		}
	}

	return nil
}

func (s *Model) handleAPIKeyInput(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(s.apiKeyInput.Value())
		if value == "" {
			return nil
		}
		// Set the env var for this session
		os.Setenv(s.apiKeyEnvVar, value)
		s.apiKeyActive = false

		// Find the auth method and trigger connection
		if s.apiKeyProviderIdx >= 0 && s.apiKeyProviderIdx < len(s.allProviders) {
			dp := &s.allProviders[s.apiKeyProviderIdx]
			if s.apiKeyAuthIdx >= 0 && s.apiKeyAuthIdx < len(dp.AuthMethods) {
				am := dp.AuthMethods[s.apiKeyAuthIdx]
				return s.connectAuthMethod(am, s.selectedIdx)
			}
		}
		return nil

	case tea.KeyEsc:
		s.apiKeyActive = false
		return nil

	default:
		var cmd tea.Cmd
		s.apiKeyInput, cmd = s.apiKeyInput.Update(key)
		return cmd
	}
}

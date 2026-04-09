package providerui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
)

// Select handles selection and returns a command.
func (s *Model) Select() tea.Cmd {
	switch {
	case s.selectorType == SelectorTypeModel:
		return s.selectModel()
	case s.tab == TabSearch:
		return s.selectSearchProvider()
	case s.level == LevelProvider:
		s.enterAuthMethodLevel()
		return nil
	default:
		return s.selectAuthMethod()
	}
}

func (s *Model) selectModel() tea.Cmd {
	if s.selectedIdx >= len(s.filteredModels) {
		return nil
	}
	selected := s.filteredModels[s.selectedIdx]
	s.active = false
	return func() tea.Msg {
		return ModelSelectedMsg{
			ModelID:      selected.ID,
			ProviderName: selected.ProviderName,
			AuthMethod:   selected.AuthMethod,
		}
	}
}

func (s *Model) selectSearchProvider() tea.Cmd {
	if s.selectedIdx >= len(s.searchProviders) {
		return nil
	}
	selected := s.searchProviders[s.selectedIdx]
	sp := search.CreateProvider(selected.Name)
	if !sp.IsAvailable() && selected.RequiresKey {
		s.lastConnectResult = "Missing: " + strings.Join(selected.EnvVars, ", ")
		s.lastConnectAuthIdx = s.selectedIdx
		s.lastConnectSuccess = false
		return nil
	}

	if s.store != nil {
		_ = s.store.SetSearchProvider(string(selected.Name))
	}
	for i := range s.searchProviders {
		if s.searchProviders[i].Status == "current" {
			s.searchProviders[i].Status = "available"
		}
	}
	s.searchProviders[s.selectedIdx].Status = "current"
	s.lastConnectResult = "\u2713 Selected"
	s.lastConnectAuthIdx = s.selectedIdx
	s.lastConnectSuccess = true

	return func() tea.Msg {
		return SearchProviderSelectedMsg{Provider: selected.Name}
	}
}

func (s *Model) enterAuthMethodLevel() {
	if s.selectedIdx >= len(s.providers) {
		return
	}
	s.parentIdx = s.selectedIdx
	s.level = LevelAuthMethod
	s.selectedIdx = 0
	s.resetConnectionResult()
}

func (s *Model) selectAuthMethod() tea.Cmd {
	if s.parentIdx >= len(s.providers) {
		return nil
	}
	authMethods := s.providers[s.parentIdx].AuthMethods
	if s.selectedIdx >= len(authMethods) {
		return nil
	}

	item := authMethods[s.selectedIdx]
	authIdx := s.selectedIdx
	if item.Status == coreprovider.StatusConnected {
		return s.disconnectAuthMethod(item, authMethods, authIdx)
	}
	return s.connectAuthMethod(item, authIdx)
}

// HandleConnectResult updates the selector state with connection result.
func (s *Model) HandleConnectResult(msg ConnectResultMsg) {
	s.lastConnectAuthIdx = msg.AuthIdx
	s.lastConnectResult = msg.Message
	s.lastConnectSuccess = msg.Success

	if !msg.Success || s.parentIdx >= len(s.providers) {
		return
	}
	authMethods := s.providers[s.parentIdx].AuthMethods
	if msg.AuthIdx < len(authMethods) {
		authMethods[msg.AuthIdx].Status = msg.NewStatus
	}
}

package providerui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
)

func (s *Model) Select() tea.Cmd {
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.visibleItems) {
		return nil
	}

	item := s.visibleItems[s.selectedIdx]
	switch item.Kind {
	case itemModel:
		return s.selectModel(item.Model)
	case itemProvider:
		return s.selectProvider(item)
	case itemAuthMethod:
		return s.selectAuthMethod(item)
	default:
		return nil
	}
}

func (s *Model) selectModel(m *modelItem) tea.Cmd {
	if m == nil {
		return nil
	}
	s.active = false
	return func() tea.Msg {
		return ModelSelectedMsg{
			ModelID:      m.ID,
			ProviderName: m.ProviderName,
			AuthMethod:   m.AuthMethod,
		}
	}
}

// selectProvider handles Enter on a provider row (Providers tab).
// Connected single auth method: refresh models.
// Disconnected single auth method: auto-connect or show API key input.
// Multiple auth methods: expand inline to show auth method list.
func (s *Model) selectProvider(item listItem) tea.Cmd {
	if item.Provider == nil {
		return nil
	}
	p := item.Provider

	if len(p.AuthMethods) == 1 {
		am := p.AuthMethods[0]
		if am.Status == coreprovider.StatusConnected {
			// Refresh: re-fetch models for this connected provider
			return s.refreshAuthMethod(am, s.selectedIdx)
		}
		return s.tryConnectOrPromptKey(am, item.ProviderIdx, 0)
	}

	if len(p.AuthMethods) == 0 {
		return nil
	}

	// Multiple auth methods: toggle inline expansion
	if s.expandedProviderIdx == item.ProviderIdx {
		s.expandedProviderIdx = -1
	} else {
		s.expandedProviderIdx = item.ProviderIdx
	}
	s.resetConnectionResult()
	s.rebuildVisibleItems()
	return nil
}

func (s *Model) selectAuthMethod(item listItem) tea.Cmd {
	if item.AuthMethod == nil {
		return nil
	}
	am := item.AuthMethod

	if am.Status == coreprovider.StatusConnected {
		// Refresh: re-fetch models for this connected auth method
		return s.refreshAuthMethod(*am, s.selectedIdx)
	}

	return s.tryConnectOrPromptKey(*am, item.ProviderIdx, s.findAuthMethodIndex(item))
}

// tryConnectOrPromptKey connects if env vars are available, otherwise shows API key input.
func (s *Model) tryConnectOrPromptKey(am authMethodItem, providerIdx, authIdx int) tea.Cmd {
	if am.Status == coreprovider.StatusAvailable || isEnvReady(am.EnvVars) {
		return s.connectAuthMethod(am, s.selectedIdx)
	}

	// Show inline API key input
	envVar := firstEnvVar(am.EnvVars)
	if envVar == "" {
		return nil
	}
	s.apiKeyProviderIdx = providerIdx
	s.apiKeyAuthIdx = authIdx
	s.initAPIKeyInput(envVar)
	return nil
}

func (s *Model) findAuthMethodIndex(item listItem) int {
	if item.AuthMethod == nil || item.ProviderIdx < 0 || item.ProviderIdx >= len(s.allProviders) {
		return 0
	}
	p := &s.allProviders[item.ProviderIdx]
	for i, am := range p.AuthMethods {
		if am.Provider == item.AuthMethod.Provider && am.AuthMethod == item.AuthMethod.AuthMethod {
			return i
		}
	}
	return 0
}

func isEnvReady(envVars []string) bool {
	for _, v := range envVars {
		if v != "" && os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

func firstEnvVar(envVars []string) string {
	for _, v := range envVars {
		if v != "" {
			return v
		}
	}
	return ""
}

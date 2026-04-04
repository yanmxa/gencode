package provider

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	coreprovider "github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Render renders the selector.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	if s.selectorType == SelectorTypeModel {
		return s.renderModelSelector()
	}

	return s.renderProviderSelector()
}

// renderModelSelector renders the model selection UI.
func (s *Model) renderModelSelector() string {
	if len(s.models) == 0 {
		return s.renderNoModelsState()
	}

	var sb strings.Builder

	title := fmt.Sprintf("Select Model (%d/%d)", len(s.filteredModels), len(s.models))
	sb.WriteString(shared.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")

	searchPrompt := "🔍 "
	if s.searchQuery == "" {
		sb.WriteString(shared.SelectorHintStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(shared.SelectorBreadcrumbStyle.Render(searchPrompt + s.searchQuery + "▏"))
	}
	sb.WriteString("\n\n")

	if len(s.filteredModels) == 0 {
		sb.WriteString(shared.SelectorHintStyle.Render("  No models match the filter"))
		sb.WriteString("\n")
	} else {
		endIdx := s.scrollOffset + s.maxVisible
		if endIdx > len(s.filteredModels) {
			endIdx = len(s.filteredModels)
		}

		if s.scrollOffset > 0 {
			sb.WriteString(shared.SelectorHintStyle.Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		currentProvider := ""
		providerHeaderStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
		warningStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
		for i := s.scrollOffset; i < endIdx; i++ {
			m := s.filteredModels[i]

			if m.ProviderName != currentProvider {
				currentProvider = m.ProviderName
				displayProvider := currentProvider
				if len(displayProvider) > 0 {
					displayProvider = strings.ToUpper(displayProvider[:1]) + displayProvider[1:]
				}
				sb.WriteString(providerHeaderStyle.Render(displayProvider) + "\n")
			}

			indicator := "[ ]"
			indicatorStyle := shared.SelectorStatusNone
			if m.IsCurrent {
				indicator = "[*]"
				indicatorStyle = shared.SelectorStatusConnected
			}

			displayName := m.DisplayName
			if displayName == "" {
				displayName = m.Name
			}
			if displayName == "" {
				displayName = m.ID
			}

			warning := ""
			if m.InputTokenLimit == 0 && m.OutputTokenLimit == 0 {
				warning = warningStyle.Render(" ⚠")
			}

			line := fmt.Sprintf("%s %s%s", indicatorStyle.Render(indicator), displayName, warning)

			if i == s.selectedIdx {
				sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredModels) {
			sb.WriteString(shared.SelectorHintStyle.Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(shared.SelectorHintStyle.Render("↑/↓ navigate · Enter select · Esc clear/cancel"))
	sb.WriteString("\n")
	warningIcon := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning).Render("⚠")
	sb.WriteString(shared.SelectorHintStyle.Render(warningIcon + " = No token limits (use /tokenlimit to set)"))

	content := sb.String()
	box := shared.SelectorBorderStyle.Width(shared.CalculateBoxWidth(s.width)).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderNoModelsState renders a styled empty state when no providers are connected.
func (s *Model) renderNoModelsState() string {
	warningStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning).Bold(true)
	msgStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text)
	cmdStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)

	content := shared.SelectorTitleStyle.Render("Select Model") + "\n\n" +
		warningStyle.Render("  ⚠  No Models Available") + "\n\n" +
		msgStyle.Render("  No LLM provider is connected yet.") + "\n" +
		msgStyle.Render("  Run ") + cmdStyle.Render("/provider") + msgStyle.Render(" to connect one first.") + "\n\n" +
		shared.SelectorHintStyle.Render("Esc cancel")

	box := shared.SelectorBorderStyle.Width(shared.CalculateBoxWidth(s.width)).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderProviderSelector renders the provider selection UI.
func (s *Model) renderProviderSelector() string {
	var sb strings.Builder

	if s.level == LevelProvider {
		sb.WriteString(s.renderTabHeader())
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(shared.SelectorTitleStyle.Render("Select Provider"))
		sb.WriteString("\n")
		if s.parentIdx < len(s.providers) {
			breadcrumb := fmt.Sprintf("› %s", s.providers[s.parentIdx].DisplayName)
			sb.WriteString(shared.SelectorBreadcrumbStyle.Render(breadcrumb))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if s.tab == TabSearch && s.level == LevelProvider {
		sb.WriteString(s.renderSearchProviders())
	} else if s.level == LevelProvider {
		for i, p := range s.providers {
			availableCount := 0
			for _, am := range p.AuthMethods {
				if am.Status == coreprovider.StatusConnected || am.Status == coreprovider.StatusAvailable {
					availableCount++
				}
			}

			statusText := ""
			if availableCount > 0 {
				statusText = fmt.Sprintf(" (%d available)", availableCount)
			}

			line := fmt.Sprintf("%s%s", p.DisplayName, shared.SelectorStatusReady.Render(statusText))

			if i == s.selectedIdx {
				sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
			}
			sb.WriteString("\n")
		}
	} else {
		if s.parentIdx < len(s.providers) {
			authMethods := s.providers[s.parentIdx].AuthMethods

			for i, am := range authMethods {
				statusIcon, statusStyle, statusDesc := getStatusDisplay(am.Status)

				line := fmt.Sprintf("%s %s %s",
					statusStyle.Render(statusIcon),
					am.DisplayName,
					shared.SelectorStatusNone.Render(statusDesc),
				)

				if i == s.selectedIdx {
					sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
				} else {
					sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
				}
				sb.WriteString("\n")

				if s.lastConnectResult != "" && i == s.lastConnectAuthIdx {
					sb.WriteString(shared.SelectorItemStyle.Render("    " + s.renderConnectResult()))
					sb.WriteString("\n")
				}
			}
		}
	}

	sb.WriteString("\n")

	if s.level == LevelProvider {
		sb.WriteString(shared.SelectorHintStyle.Render("Tab switch · ↑/↓ navigate · Enter select · Esc cancel"))
	} else {
		sb.WriteString(shared.SelectorHintStyle.Render("↑/↓ navigate · Enter select · ←/Esc back"))
	}

	content := sb.String()
	box := shared.SelectorBorderStyle.Width(shared.CalculateBoxWidth(s.width)).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// renderTabHeader renders the tab header for the provider selector.
func (s *Model) renderTabHeader() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	var llmTab, searchTab string

	if s.tab == TabLLM {
		llmTab = activeStyle.Render("[LLM]")
		searchTab = inactiveStyle.Render(" Search ")
	} else {
		llmTab = inactiveStyle.Render(" LLM ")
		searchTab = activeStyle.Render("[Search]")
	}

	tabs := llmTab + "  " + searchTab
	boxWidth := shared.CalculateBoxWidth(s.width)
	return lipgloss.PlaceHorizontal(boxWidth-4, lipgloss.Center, tabs)
}

// getSearchProviderStatus returns icon, style, and description for a search provider.
func getSearchProviderStatus(status string, requiresKey bool) (icon string, style lipgloss.Style, desc string) {
	switch status {
	case "current":
		return "●", shared.SelectorStatusConnected, ""
	case "available":
		return "○", shared.SelectorStatusReady, ""
	default:
		if requiresKey {
			return "◌", shared.SelectorStatusNone, "(no credentials)"
		}
		return "◌", shared.SelectorStatusNone, ""
	}
}

// renderSearchProviders renders the search provider list.
func (s *Model) renderSearchProviders() string {
	var sb strings.Builder

	for i, sp := range s.searchProviders {
		statusIcon, statusStyle, statusDesc := getSearchProviderStatus(sp.Status, sp.RequiresKey)

		line := fmt.Sprintf("%s %s %s",
			statusStyle.Render(statusIcon),
			sp.DisplayName,
			shared.SelectorStatusNone.Render(statusDesc),
		)

		if i == s.selectedIdx {
			sb.WriteString(shared.SelectorSelectedStyle.Render("> " + line))
		} else {
			sb.WriteString(shared.SelectorItemStyle.Render("  " + line))
		}
		sb.WriteString("\n")

		if s.lastConnectResult != "" && i == s.lastConnectAuthIdx {
			sb.WriteString(shared.SelectorItemStyle.Render("    " + s.renderConnectResult()))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// connectResultStyle returns the appropriate style for a connection result message.
func (s *Model) connectResultStyle() lipgloss.Style {
	if !s.lastConnectSuccess {
		if s.lastConnectResult == "Connecting..." {
			return shared.SelectorHintStyle
		}
		return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Error)
	}
	if strings.HasPrefix(s.lastConnectResult, "⚠") {
		return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)
	}
	return shared.SelectorStatusConnected
}

// renderConnectResult returns the styled result message for connection attempts.
func (s *Model) renderConnectResult() string {
	return s.connectResultStyle().Render(s.lastConnectResult)
}

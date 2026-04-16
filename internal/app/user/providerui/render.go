package providerui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// Render renders the unified model & provider selector as a full-screen overlay.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	if len(s.visibleItems) == 0 && len(s.allModels) == 0 && len(s.allProviders) == 0 {
		return s.renderEmptyState()
	}

	var sb strings.Builder

	sepStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	sepWidth := s.contentWidth() - 8

	// Separator above tabs
	sb.WriteString(sepStyle.Render(strings.Repeat("─", sepWidth)))
	sb.WriteString("\n")

	// Tab header
	sb.WriteString(s.renderTabs())
	sb.WriteString("\n\n")

	// Search box
	sb.WriteString(s.renderSearchBox())
	sb.WriteString("\n\n")

	if len(s.visibleItems) == 0 {
		sb.WriteString(s.emptyFilterMsg())
		sb.WriteString("\n")
	} else {
		s.renderItemList(&sb)
	}

	// Separator before hints
	sb.WriteString("\n")
	sb.WriteString(sepStyle.Render(strings.Repeat("─", sepWidth)))
	sb.WriteString("\n")
	sb.WriteString(s.renderHints())

	content := sb.String()
	cw := s.contentWidth()
	box := lipgloss.NewStyle().
		Width(cw).
		Height(s.boxHeight()).
		Padding(1, 2).
		Render(content)
	return lipgloss.Place(s.width, s.height-2, lipgloss.Center, lipgloss.Top, box)
}

// contentWidth returns the usable width for the panel content.
func (s *Model) contentWidth() int {
	// Use full terminal width minus a small margin
	return max(60, s.width-6)
}

// boxHeight returns the fixed height for the panel, consistent across tabs.
func (s *Model) boxHeight() int {
	return max(18, s.height-4)
}

// emptyFilterMsg returns the "no matches" text for the current tab.
func (s *Model) emptyFilterMsg() string {
	if s.activeTab == tabModels {
		return kit.DimStyle().PaddingLeft(2).Render("No models match the filter")
	}
	return kit.DimStyle().PaddingLeft(2).Render("No providers match the filter")
}

// renderItemList renders the scrollable item list into the builder.
func (s *Model) renderItemList(sb *strings.Builder) {
	endIdx := min(s.scrollOffset+s.maxVisible, len(s.visibleItems))

	if s.scrollOffset > 0 {
		sb.WriteString(kit.DimStyle().PaddingLeft(2).Render("↑ more above"))
		sb.WriteString("\n")
	}

	for i := s.scrollOffset; i < endIdx; i++ {
		item := s.visibleItems[i]
		isSelected := i == s.selectedIdx

		switch item.Kind {
		case itemProviderHeader:
			sb.WriteString(s.renderProviderHeader(item))
		case itemModel:
			sb.WriteString(s.renderModelRow(item, isSelected))
		case itemProvider:
			sb.WriteString(s.renderProviderRow(item, isSelected, i))
		case itemAuthMethod:
			sb.WriteString(s.renderAuthMethod(item, isSelected, i))
		}
		sb.WriteString("\n")

		// Inline API key input (render below the relevant item)
		if s.apiKeyActive && isSelected {
			sb.WriteString(s.renderAPIKeyInput())
			sb.WriteString("\n")
		}
	}

	if endIdx < len(s.visibleItems) {
		sb.WriteString(kit.DimStyle().PaddingLeft(2).Render("↓ more below"))
		sb.WriteString("\n")
	}
}

// ── Tab header ──────────────────────────────────────────────────────────────


func (s *Model) renderTabs() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(kit.TabActiveFg).
		Background(kit.TabActiveBg).
		Bold(true).
		Padding(0, 2)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim).
		Padding(0, 2)

	tabs := []struct {
		name string
		tab  tab
	}{
		{"Models", tabModels},
		{"Providers", tabProviders},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == s.activeTab {
			parts = append(parts, activeStyle.Render(t.name))
		} else {
			parts = append(parts, inactiveStyle.Render(t.name))
		}
	}

	return strings.Join(parts, "  ")
}

// ── Search box ──────────────────────────────────────────────────────────────

func (s *Model) renderSearchBox() string {
	innerWidth := max(20, s.contentWidth()-8)

	var text string
	if s.activeTab == tabModels && s.searchQuery != "" {
		totalModels := len(s.allModels)
		filteredCount := len(s.filteredModels)
		text = fmt.Sprintf(" 🔍 %s▏ (%d/%d)", s.searchQuery, filteredCount, totalModels)
	} else if s.searchQuery != "" {
		text = " 🔍 " + s.searchQuery + "▏"
	} else {
		if s.activeTab == tabProviders {
			text = " 🔍 Type to filter providers..."
		} else {
			text = " 🔍 Type to filter models..."
		}
	}

	textFg := kit.CurrentTheme.TextDim
	if s.searchQuery != "" {
		textFg = kit.CurrentTheme.Text
	}

	searchBg := kit.SearchBg
	return lipgloss.NewStyle().
		Foreground(textFg).
		Background(searchBg).
		Padding(0, 1).
		Width(innerWidth).
		Render(text)
}

// ── Empty / no providers ────────────────────────────────────────────────────

func (s *Model) renderEmptyState() string {
	warningStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning).Bold(true)
	msgStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Text)
	cmdStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)

	content := s.renderTabs() + "\n\n" +
		s.renderSearchBox() + "\n\n" +
		warningStyle.Render("  ⚠  No Models Available") + "\n\n" +
		msgStyle.Render("  No LLM provider is connected yet.") + "\n" +
		msgStyle.Render("  Press ") + cmdStyle.Render("Tab") + msgStyle.Render(" to switch to Providers tab and connect one.") + "\n\n" +
		kit.DimStyle().Render("←/→/Tab switch · Esc cancel")

	cw := s.contentWidth()
	box := lipgloss.NewStyle().
		Width(cw).
		Height(s.boxHeight()).
		Padding(1, 2).
		Render(content)
	return lipgloss.Place(s.width, s.height-2, lipgloss.Center, lipgloss.Top, box)
}

// ── Models tab rows ─────────────────────────────────────────────────────────

func (s *Model) renderProviderHeader(item listItem) string {
	style := lipgloss.NewStyle().
		Foreground(kit.CurrentTheme.TextDim).
		Bold(true)
	name := item.Provider.DisplayName
	if name == "" {
		name = string(item.Provider.Provider)
	}
	return style.Render(name)
}

func (s *Model) renderModelRow(item listItem, isSelected bool) string {
	m := item.Model

	indicator := "[ ]"
	indicatorStyle := kit.SelectorStatusNone()
	if m.IsCurrent {
		indicator = "[*]"
		indicatorStyle = kit.SelectorStatusConnected()
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
		warning = lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning).Render(" ⚠")
	}

	line := fmt.Sprintf("%s %s%s", indicatorStyle.Render(indicator), displayName, warning)
	return kit.RenderSelectableRow(line, isSelected)
}

// ── Providers tab rows ──────────────────────────────────────────────────────

// providerNameColumnWidth is the fixed width for provider name alignment.
const providerNameColumnWidth = 16

func (s *Model) renderProviderRow(item listItem, isSelected bool, itemIdx int) string {
	p := item.Provider
	if p == nil {
		return ""
	}

	bestStatus := bestAuthMethodStatus(p.AuthMethods)
	statusIcon, statusStyle, _ := getStatusDisplay(bestStatus)

	envInfo := ""
	if len(p.AuthMethods) == 1 {
		envInfo = kit.RenderEnvVarStatus(firstEnvVar(p.AuthMethods[0].EnvVars))
	} else if len(p.AuthMethods) > 1 {
		envInfo = kit.DimStyle().Render(fmt.Sprintf("%d auth methods", len(p.AuthMethods)))
	}

	line := kit.FormatAlignedRow(statusStyle.Render(statusIcon), p.DisplayName, providerNameColumnWidth, envInfo)
	result := kit.RenderSelectableRow(line, isSelected)

	if s.lastConnectResult != "" && itemIdx == s.lastConnectAuthIdx {
		result += "\n" + resultIndent + s.renderConnectResult()
	}

	return result
}

func (s *Model) renderAuthMethod(item listItem, isSelected bool, itemIdx int) string {
	am := item.AuthMethod
	if am == nil {
		return ""
	}

	statusIcon, statusStyle, statusDesc := getStatusDisplay(am.Status)

	envInfo := ""
	if am.Status != llm.StatusConnected {
		envInfo = kit.RenderEnvVarStatus(firstEnvVar(am.EnvVars))
	}
	if statusDesc != "" && envInfo == "" {
		envInfo = kit.DimStyle().Render(statusDesc)
	}

	colWidth := providerNameColumnWidth - 2 // sub-item indent
	line := "  " + kit.FormatAlignedRow(statusStyle.Render(statusIcon), am.DisplayName, colWidth, envInfo)
	result := kit.RenderSelectableRow(line, isSelected)

	if s.lastConnectResult != "" && itemIdx == s.lastConnectAuthIdx {
		result += "\n" + resultIndent + "  " + s.renderConnectResult()
	}

	return result
}

// ── API key input ───────────────────────────────────────────────────────────

func (s *Model) renderAPIKeyInput() string {
	label := kit.DimStyle().Render(s.apiKeyEnvVar + ": ")
	inputView := label + s.apiKeyInput.View()

	inputBg := lipgloss.AdaptiveColor{Dark: "#1E293B", Light: "#F1F5F9"}
	boxStyle := lipgloss.NewStyle().
		Background(inputBg).
		Padding(0, 1)

	// Indent to align with auth method content (6 chars: PaddingLeft(2) + "  " + "  ")
	return "      " + boxStyle.Render(inputView)
}

// ── Footer hints ────────────────────────────────────────────────────────────

func (s *Model) renderHints() string {
	if s.apiKeyActive {
		return kit.DimStyle().Render("Paste API key · Enter confirm · Esc cancel")
	}

	var parts []string
	parts = append(parts, "↑/↓ navigate")
	if s.activeTab == tabProviders {
		parts = append(parts, "Enter connect/refresh")
	} else {
		parts = append(parts, "Enter select")
	}
	parts = append(parts, "←/→/Tab switch", "Esc cancel")
	return kit.DimStyle().Render(strings.Join(parts, " · "))
}

// ── Connection result ───────────────────────────────────────────────────────

// resultIndent is the fixed indent for connection result messages,
// aligned with the provider name column.
const resultIndent = "        "

func (s *Model) connectResultStyle() lipgloss.Style {
	if !s.lastConnectSuccess {
		if s.lastConnectResult == "Connecting..." || s.lastConnectResult == "Refreshing..." {
			return kit.DimStyle()
		}
		return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
	}
	if strings.HasPrefix(s.lastConnectResult, "⚠") {
		return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Warning)
	}
	return kit.SelectorStatusConnected()
}

func (s *Model) renderConnectResult() string {
	return s.connectResultStyle().Render(s.lastConnectResult)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func bestAuthMethodStatus(methods []authMethodItem) llm.Status {
	for _, m := range methods {
		if m.Status == llm.StatusConnected {
			return llm.StatusConnected
		}
	}
	for _, m := range methods {
		if m.Status == llm.StatusAvailable {
			return llm.StatusAvailable
		}
	}
	return llm.StatusNotConfigured
}

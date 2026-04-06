package plugin

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Render renders the plugin selector
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	switch s.level {
	case LevelDetail:
		if s.detailPlugin != nil {
			return s.renderInstalledDetail()
		}
		if s.detailDiscover != nil {
			return s.renderDiscoverDetail()
		}
		if s.detailMarketplace != nil {
			return s.renderMarketplaceDetail()
		}
	case LevelAddMarketplace:
		return s.renderAddMarketplaceDialog()
	case LevelBrowsePlugins:
		return s.renderBrowsePlugins()
	}

	return s.renderTabList()
}

// renderTabs renders the tab navigation bar like Claude Code
func (s *Model) renderTabs() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Bold(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)
	separatorStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextDim)

	tabs := []struct {
		name string
		tab  Tab
	}{
		{"Discover", TabDiscover},
		{"Installed", TabInstalled},
		{"Marketplaces", TabMarketplaces},
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == s.activeTab {
			parts = append(parts, activeStyle.Render(t.name))
		} else {
			parts = append(parts, inactiveStyle.Render(t.name))
		}
	}

	return strings.Join(parts, separatorStyle.Render("  |  "))
}

// renderTabList renders the main tab list view
func (s *Model) renderTabList() string {
	var sb strings.Builder

	sb.WriteString(shared.SelectorTitleStyle.Render("Plugin Manager"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render(s.renderTabs()))
	sb.WriteString("\n\n")

	s.renderSearchBox(&sb)
	sb.WriteString("\n\n")

	switch s.activeTab {
	case TabInstalled:
		s.renderInstalledList(&sb)
	case TabDiscover:
		s.renderDiscoverList(&sb)
	case TabMarketplaces:
		s.renderMarketplacesList(&sb)
	}

	hint := s.getTabHint()
	s.renderFooter(&sb, hint)
	return s.renderBox(sb.String())
}

// getItemCount returns current position and total count for the active tab
func (s *Model) getItemCount() (int, int) {
	total := len(s.filteredItems)
	if s.activeTab == TabMarketplaces {
		total++
	}
	pos := s.selectedIdx + 1
	if total == 0 {
		pos = 0
	}
	return pos, total
}

// renderSearchBox renders the search input
func (s *Model) renderSearchBox(sb *strings.Builder) {
	searchStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	inputStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	pos, total := s.getItemCount()
	countText := fmt.Sprintf("  %d/%d", pos, total)

	if s.searchQuery == "" {
		sb.WriteString(searchStyle.Render("⌕ Search..."))
		sb.WriteString(searchStyle.Render(countText))
	} else {
		sb.WriteString(searchStyle.Render("⌕ "))
		sb.WriteString(inputStyle.Render(s.searchQuery))
		sb.WriteString(inputStyle.Render("│"))
		sb.WriteString(searchStyle.Render(countText))
	}
}

func (s *Model) getTabHint() string {
	switch s.activeTab {
	case TabInstalled:
		return "↑↓ navigate · space toggle · enter details · esc close"
	case TabDiscover:
		return "↑↓ navigate · enter details · esc close"
	case TabMarketplaces:
		return "↑↓ navigate · u update · r remove · esc close"
	}
	return ""
}

func (s *Model) renderInstalledList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.installedFlatList) == 0 {
			sb.WriteString(dimStyle.Render("No plugins installed"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Run: gen plugin install <name>@<marketplace>"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("No matches"))
			sb.WriteString("\n")
		}
		return
	}

	endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(PluginItem)
		if !ok {
			continue
		}

		icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(p.Name)

		if p.Marketplace != "" {
			sb.WriteString(dimStyle.Render(" · " + p.Marketplace))
		}

		if p.Description != "" {
			prefixLen := 4 + len(p.Name) + 3 + len(p.Marketplace)
			maxDescLen := s.width - prefixLen - 5
			if maxDescLen > 20 {
				desc := shared.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.Render(" · " + desc))
			}
		}
		sb.WriteString("\n")
	}

	if s.scrollOffset > 0 || endIdx < len(s.filteredItems) {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  (%d more)", len(s.filteredItems)-s.maxVisible)))
		sb.WriteString("\n")
	}
}

func (s *Model) renderDiscoverList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.discoverPlugins) == 0 {
			sb.WriteString(dimStyle.Render("No plugins available"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("Add a marketplace in the Marketplaces tab"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("No matches"))
			sb.WriteString("\n")
		}
		return
	}

	maxItems := s.maxVisible / 2
	if maxItems < 3 {
		maxItems = 3
	}
	endIdx := min(s.scrollOffset+maxItems, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(DiscoverPluginItem)
		if !ok {
			continue
		}

		icon := "○"
		iconStyle := dimStyle
		if p.Installed {
			icon = "●"
			iconStyle = shared.SelectorStatusConnected
		}

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(p.Name)
		sb.WriteString(dimStyle.Render(" · " + p.Marketplace))
		sb.WriteString("\n")

		if p.Description != "" {
			maxDescLen := s.width - 8
			if maxDescLen > 100 {
				maxDescLen = 100
			}
			if maxDescLen > 20 {
				desc := shared.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.Render("    " + desc))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
	}

	if s.scrollOffset > 0 || endIdx < len(s.filteredItems) {
		remaining := len(s.filteredItems) - endIdx
		if remaining > 0 {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("  (%d more)", remaining)))
			sb.WriteString("\n")
		}
	}
}

func (s *Model) renderMarketplacesList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	addStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)

	sb.WriteString(pluginCursor(s.selectedIdx == 0))
	sb.WriteString(addStyle.Render("+ Add Marketplace"))
	sb.WriteString("\n")

	if len(s.filteredItems) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("No marketplaces configured"))
		sb.WriteString("\n")
		return
	}

	sb.WriteString("\n")

	endIdx := min(s.scrollOffset+s.maxVisible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		m, ok := s.filteredItems[i].(MarketplaceItem)
		if !ok {
			continue
		}

		displayIdx := i + 1
		official := ""
		if m.IsOfficial {
			official = " ✻"
		}

		sb.WriteString(pluginCursor(displayIdx == s.selectedIdx))
		sb.WriteString(shared.SelectorStatusConnected.Render("●"))
		sb.WriteString(" ")
		sb.WriteString(m.ID)
		sb.WriteString(dimStyle.Render(official))
		sb.WriteString("\n")

		if displayIdx == s.selectedIdx {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("    %s", m.Source)))
			sb.WriteString("\n")
			stats := fmt.Sprintf("    %d available · %d installed", m.Available, m.Installed)
			if m.LastUpdated != "" {
				stats += " · " + m.LastUpdated
			}
			sb.WriteString(dimStyle.Render(stats))
			sb.WriteString("\n")
		}
	}
}

func (s *Model) renderInstalledDetail() string {
	if s.detailPlugin == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailPlugin
	maxValueLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Plugin Details"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render(p.FullName))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(p.FullName))
	sb.WriteString("\n\n")

	icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)
	statusLabel := "Disabled"
	if p.Enabled {
		statusLabel = "Enabled"
	}
	sb.WriteString(dimStyle.Render("Status:  "))
	sb.WriteString(iconStyle.Render(icon + " " + statusLabel))
	sb.WriteString("\n")

	sb.WriteString(dimStyle.Render("Scope:   "))
	sb.WriteString(brightStyle.Render(string(p.Scope)))
	sb.WriteString("\n")

	if p.Version != "" {
		sb.WriteString(dimStyle.Render("Version: "))
		sb.WriteString(brightStyle.Render(p.Version))
		sb.WriteString("\n")
	}

	if p.Author != "" {
		sb.WriteString(dimStyle.Render("Author:  "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n")
	}

	if p.Description != "" {
		sb.WriteString("\n")
		desc := shared.TruncateText(p.Description, maxValueLen)
		sb.WriteString(dimStyle.Render(desc))
		sb.WriteString("\n")
	}

	components := buildComponentList(p)
	if len(components) > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("Components: " + strings.Join(components, ", ")))
		sb.WriteString("\n")
	}

	if len(p.Errors) > 0 {
		sb.WriteString("\n")
		sb.WriteString(shared.SelectorStatusError.Render("Errors:"))
		sb.WriteString("\n")
		for _, err := range p.Errors {
			sb.WriteString(shared.SelectorStatusError.Render("  • " + shared.TruncateText(err, maxValueLen)))
			sb.WriteString("\n")
		}
	}

	s.renderActions(&sb)
	s.renderFooter(&sb, "↑↓ navigate · enter select · esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderDiscoverDetail() string {
	if s.detailDiscover == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	p := s.detailDiscover
	maxValueLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)
	warnStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)

	sb.WriteString(shared.SelectorTitleStyle.Render("Install Plugin"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render(p.Name + "@" + p.Marketplace))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(p.Name))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("from " + p.Marketplace))
	sb.WriteString("\n\n")

	if p.Description != "" {
		desc := shared.TruncateText(p.Description, maxValueLen)
		sb.WriteString(dimStyle.Render(desc))
		sb.WriteString("\n\n")
	}

	if p.Author != "" {
		sb.WriteString(dimStyle.Render("By: "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n\n")
	}

	sb.WriteString(warnStyle.Render("⚠ Make sure you trust a plugin before installing"))
	sb.WriteString("\n\n")

	s.renderActions(&sb)
	s.renderFooter(&sb, "enter select · esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderMarketplaceDetail() string {
	if s.detailMarketplace == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	m := s.detailMarketplace

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Marketplace Details"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render(m.ID))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(m.ID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(m.Source))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("%d available plugins", m.Available))
	sb.WriteString("\n")

	if m.Installed > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("Installed (%d):", m.Installed)))
		sb.WriteString("\n")
		for _, p := range coreplugin.DefaultRegistry.List() {
			if idx := strings.Index(p.Source, "@"); idx != -1 && p.Source[idx+1:] == m.ID {
				sb.WriteString("  ● " + p.Name())
				sb.WriteString("\n")
			}
		}
	}

	s.renderActions(&sb)
	s.renderFooter(&sb, "enter select · esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderAddMarketplaceDialog() string {
	var sb strings.Builder
	maxInputLen := s.width - 20

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Add Marketplace"))
	sb.WriteString("\n\n")

	sb.WriteString(dimStyle.Render("Enter marketplace source:"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Examples:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • https://github.com/owner/repo"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • owner/repo (GitHub shorthand)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  • ./path/to/marketplace (local)"))
	sb.WriteString("\n\n")

	inputLine := s.addMarketplaceInput + "│"
	if len(inputLine) > maxInputLen {
		inputLine = "…" + inputLine[len(inputLine)-maxInputLen+1:]
	}
	sb.WriteString(brightStyle.Render("> " + inputLine))
	sb.WriteString("\n")

	s.renderFooter(&sb, "enter add · esc cancel")
	return s.renderBox(sb.String())
}

func (s *Model) renderBrowsePlugins() string {
	var sb strings.Builder
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Browse Marketplace"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render(s.browseMarketplaceID))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(s.browseMarketplaceID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("%d available plugins", len(s.browsePlugins))))
	sb.WriteString("\n\n")

	if len(s.browsePlugins) == 0 {
		sb.WriteString(dimStyle.Render("No plugins found"))
		sb.WriteString("\n")
	} else {
		endIdx := min(s.scrollOffset+s.maxVisible, len(s.browsePlugins))

		for i := s.scrollOffset; i < endIdx; i++ {
			p := s.browsePlugins[i]

			icon := "○"
			iconStyle := dimStyle
			if p.Installed {
				icon = "●"
				iconStyle = shared.SelectorStatusConnected
			}

			sb.WriteString(pluginCursor(i == s.selectedIdx))
			sb.WriteString(iconStyle.Render(icon))
			sb.WriteString(" ")
			sb.WriteString(p.Name)
			sb.WriteString("\n")

			if p.Description != "" && i == s.selectedIdx {
				desc := shared.TruncateText(p.Description, s.width-10)
				sb.WriteString(dimStyle.Render("    " + desc))
				sb.WriteString("\n")
			}
		}
	}

	s.renderFooter(&sb, "↑↓ navigate · enter details · esc back")
	return s.renderBox(sb.String())
}

// renderActions renders the action list for detail views
func (s *Model) renderActions(sb *strings.Builder) {
	sb.WriteString("\n")
	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(shared.SelectorSelectedStyle.Render("› " + action.Label))
		} else {
			sb.WriteString(shared.SelectorItemStyle.Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}
}

func (s *Model) renderFooter(sb *strings.Builder, hint string) {
	sb.WriteString("\n")
	if s.isLoading {
		spinnerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent)
		sb.WriteString(spinnerStyle.Render("  ◐ " + s.loadingMsg))
		sb.WriteString("\n\n")
	} else if s.lastMessage != "" {
		if s.isError {
			sb.WriteString(shared.SelectorStatusError.Render("  ⚠ " + s.lastMessage))
		} else {
			successStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)
			sb.WriteString(successStyle.Render("  ✓ " + s.lastMessage))
		}
		sb.WriteString("\n\n")
	}
	sb.WriteString(s.renderHints(hint))
}

// renderHints renders keyboard hints in a clean format
func (s *Model) renderHints(hint string) string {
	hintStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
	return hintStyle.Render(hint)
}

func (s *Model) renderBox(content string) string {
	box := shared.SelectorBorderStyle.Width(shared.CalculateBoxWidth(s.width)).Render(content)
	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

func pluginStatusIconAndStyle(enabled bool) (string, lipgloss.Style) {
	if enabled {
		return "●", shared.SelectorStatusConnected
	}
	return "○", shared.SelectorStatusNone
}

func pluginCursor(selected bool) string {
	if selected {
		return "❯ "
	}
	return "  "
}

// buildComponentList builds a list of component counts for display
func buildComponentList(p *PluginItem) []string {
	type componentCount struct {
		name  string
		count int
	}
	counts := []componentCount{
		{"Skills", p.Skills},
		{"Agents", p.Agents},
		{"Commands", p.Commands},
		{"Hooks", p.Hooks},
		{"MCP", p.MCP},
		{"LSP", p.LSP},
	}

	var result []string
	for _, c := range counts {
		if c.count > 0 {
			result = append(result, fmt.Sprintf("%s: %d", c.name, c.count))
		}
	}
	return result
}

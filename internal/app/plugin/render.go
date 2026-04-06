package plugin

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/ui/shared"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

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

func (s *Model) boxWidth() int {
	return max(80, s.width-4)
}

func (s *Model) boxHeight() int {
	return max(18, s.height-6)
}

func (s *Model) contentWidth() int {
	return s.boxWidth() - 6
}

func (s *Model) bodyHeight() int {
	return max(6, s.boxHeight()-8)
}

func (s *Model) renderTabs() string {
	activeStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

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
			parts = append(parts, activeStyle.Render("["+t.name+"]"))
		} else {
			parts = append(parts, inactiveStyle.Render(" "+t.name+" "))
		}
	}

	return strings.Join(parts, " ")
}

func (s *Model) renderTabList() string {
	var sb strings.Builder
	var body strings.Builder

	pos, total := s.getItemCount()
	title := fmt.Sprintf("Plugin Manager (%d/%d)", pos, total)
	sb.WriteString(shared.SelectorTitleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(s.renderTabs())
	sb.WriteString("\n\n")

	s.renderSearchBox(&sb)
	sb.WriteString("\n\n")

	switch s.activeTab {
	case TabInstalled:
		s.renderInstalledList(&body)
	case TabDiscover:
		s.renderDiscoverList(&body)
	case TabMarketplaces:
		s.renderMarketplacesList(&body)
	}

	sb.WriteString(s.renderViewport(body.String(), 0))

	hint := s.getTabHint()
	s.renderFooter(&sb, hint)
	return s.renderBox(sb.String())
}

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

func (s *Model) renderSearchBox(sb *strings.Builder) {
	searchStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	inputStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	searchPrompt := "\U0001f50d "
	if s.searchQuery == "" {
		sb.WriteString(searchStyle.Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(searchStyle.Render(searchPrompt))
		sb.WriteString(inputStyle.Render(s.searchQuery))
		sb.WriteString(inputStyle.Render("\u258f"))
	}
}

func (s *Model) getTabHint() string {
	switch s.activeTab {
	case TabInstalled:
		return "\u2190/\u2192 tabs \u00b7 \u2191/\u2193 navigate \u00b7 space toggle \u00b7 enter details \u00b7 esc close"
	case TabDiscover:
		return "\u2190/\u2192 tabs \u00b7 \u2191/\u2193 navigate \u00b7 enter details \u00b7 esc close"
	case TabMarketplaces:
		return "\u2190/\u2192 tabs \u00b7 \u2191/\u2193 navigate \u00b7 u update \u00b7 r remove \u00b7 esc close"
	}
	return ""
}

func (s *Model) renderInstalledList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.installedFlatList) == 0 {
			sb.WriteString(dimStyle.Render("  No plugins installed"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("  Run: gen plugin install <name>@<marketplace>"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("  No plugins match the filter"))
			sb.WriteString("\n")
		}
		return
	}

	visible := max(4, s.bodyHeight())
	endIdx := min(s.scrollOffset+visible, len(s.filteredItems))
	cw := s.contentWidth()

	maxNameWidth := 0
	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(PluginItem)
		if !ok {
			continue
		}
		nameLen := len(p.Name)
		if p.Marketplace != "" {
			nameLen += len(p.Marketplace) + 3
		}
		if nameLen > maxNameWidth {
			maxNameWidth = nameLen
		}
	}
	if maxNameWidth > 35 {
		maxNameWidth = 35
	}

	if s.scrollOffset > 0 {
		sb.WriteString(shared.SelectorHintStyle.Render("  \u2191 more above"))
		sb.WriteString("\n")
	}

	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(PluginItem)
		if !ok {
			continue
		}

		icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)

		nameStr := p.Name
		if p.Marketplace != "" {
			nameStr += dimStyle.Render(" \u00b7 " + p.Marketplace)
		}

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(nameStr)

		if p.Description != "" {
			rawNameLen := len(p.Name)
			if p.Marketplace != "" {
				rawNameLen += 3 + len(p.Marketplace)
			}
			prefixLen := 4 + rawNameLen
			maxDescLen := cw - prefixLen - 2
			if maxDescLen > 20 {
				desc := shared.TruncateText(p.Description, maxDescLen)
				sb.WriteString(dimStyle.Render(" \u00b7 " + desc))
			}
		}
		sb.WriteString("\n")
	}

	if endIdx < len(s.filteredItems) {
		sb.WriteString(shared.SelectorHintStyle.Render("  \u2193 more below"))
		sb.WriteString("\n")
	}
}

func (s *Model) renderDiscoverList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)

	if len(s.filteredItems) == 0 {
		if len(s.discoverPlugins) == 0 {
			sb.WriteString(dimStyle.Render("  No plugins available"))
			sb.WriteString("\n\n")
			sb.WriteString(dimStyle.Render("  Add a marketplace in the Marketplaces tab"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(dimStyle.Render("  No plugins match the filter"))
			sb.WriteString("\n")
		}
		return
	}

	maxItems := max(3, s.bodyHeight()/3)
	endIdx := min(s.scrollOffset+maxItems, len(s.filteredItems))

	if s.scrollOffset > 0 {
		sb.WriteString(shared.SelectorHintStyle.Render("  \u2191 more above"))
		sb.WriteString("\n")
	}

	cw := s.contentWidth()
	for i := s.scrollOffset; i < endIdx; i++ {
		p, ok := s.filteredItems[i].(DiscoverPluginItem)
		if !ok {
			continue
		}

		icon := "\u25cb"
		iconStyle := dimStyle
		if p.Installed {
			icon = "\u25cf"
			iconStyle = shared.SelectorStatusConnected
		}

		sb.WriteString(pluginCursor(i == s.selectedIdx))
		sb.WriteString(iconStyle.Render(icon))
		sb.WriteString(" ")
		sb.WriteString(p.Name)
		sb.WriteString(dimStyle.Render(" \u00b7 " + p.Marketplace))
		sb.WriteString("\n")

		if p.Description != "" {
			maxDescLen := cw - 8
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

	remaining := len(s.filteredItems) - endIdx
	if remaining > 0 {
		sb.WriteString(shared.SelectorHintStyle.Render(fmt.Sprintf("  \u2193 %d more below", remaining)))
		sb.WriteString("\n")
	}
}

func (s *Model) renderMarketplacesList(sb *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	addStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success).Bold(true)

	sb.WriteString(pluginCursor(s.selectedIdx == 0))
	sb.WriteString(addStyle.Render("+ Add Marketplace"))
	sb.WriteString("\n")

	if len(s.filteredItems) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  No marketplaces configured"))
		sb.WriteString("\n")
		return
	}

	sb.WriteString("\n")

	visible := max(4, s.bodyHeight()/2)
	endIdx := min(s.scrollOffset+visible, len(s.filteredItems))

	for i := s.scrollOffset; i < endIdx; i++ {
		m, ok := s.filteredItems[i].(MarketplaceItem)
		if !ok {
			continue
		}

		displayIdx := i + 1
		official := ""
		if m.IsOfficial {
			official = " \u272b"
		}

		sb.WriteString(pluginCursor(displayIdx == s.selectedIdx))
		sb.WriteString(shared.SelectorStatusConnected.Render("\u25cf"))
		sb.WriteString(" ")
		sb.WriteString(m.ID)
		sb.WriteString(dimStyle.Render(official))
		sb.WriteString("\n")

		if displayIdx == s.selectedIdx {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("    %s", m.Source)))
			sb.WriteString("\n")
			stats := fmt.Sprintf("    %d available \u00b7 %d installed", m.Available, m.Installed)
			if m.LastUpdated != "" {
				stats += " \u00b7 " + m.LastUpdated
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
	cw := s.contentWidth()

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)
	labelStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Width(12)

	icon, iconStyle := pluginStatusIconAndStyle(p.Enabled)
	statusLabel := "Disabled"
	if p.Enabled {
		statusLabel = "Enabled"
	}
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Status:"), iconStyle.Render(icon+" "+statusLabel)))
	sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Scope:"), brightStyle.Render(string(p.Scope))))

	if p.Version != "" {
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Version:"), brightStyle.Render(p.Version)))
	}

	if p.Author != "" {
		sb.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Author:"), brightStyle.Render(p.Author)))
	}

	if p.Description != "" {
		sb.WriteString("\n")
		desc := shared.TruncateText(p.Description, cw-4)
		sb.WriteString("  " + dimStyle.Render(desc))
		sb.WriteString("\n")
	}

	components := buildComponentList(p)
	if len(components) > 0 {
		sb.WriteString("\n")
		compLabel := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text).Bold(true)
		sb.WriteString("  " + compLabel.Render("Components"))
		sb.WriteString("\n")
		for _, c := range components {
			sb.WriteString("  " + dimStyle.Render("  \u2022 "+c))
			sb.WriteString("\n")
		}
	}

	if len(p.Errors) > 0 {
		sb.WriteString("\n")
		sb.WriteString("  " + shared.SelectorStatusError.Render("Errors"))
		sb.WriteString("\n")
		maxValueLen := cw - 8
		for _, err := range p.Errors {
			sb.WriteString("  " + shared.SelectorStatusError.Render("  \u2022 "+shared.TruncateText(err, maxValueLen)))
			sb.WriteString("\n")
		}
	}

	content := sb.String()
	var frame strings.Builder
	frame.WriteString(shared.SelectorTitleStyle.Render("Plugin Details"))
	frame.WriteString("\n")
	frame.WriteString(shared.SelectorBreadcrumbStyle.Render("> " + p.FullName))
	frame.WriteString("\n\n")
	frame.WriteString(s.renderViewport(content, s.detailScroll))
	s.renderActions(&frame)
	s.renderFooter(&frame, "\u2191/\u2193 scroll/actions \u00b7 enter select \u00b7 esc back")
	return s.renderBox(frame.String())
}

func (s *Model) renderDiscoverDetail() string {
	if s.detailDiscover == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	var frame strings.Builder
	p := s.detailDiscover
	cw := s.contentWidth()

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)
	warnStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Warning)

	sb.WriteString(brightStyle.Render(p.Name))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("from " + p.Marketplace))
	sb.WriteString("\n\n")

	if p.Description != "" {
		desc := shared.TruncateText(p.Description, cw-4)
		sb.WriteString("  " + dimStyle.Render(desc))
		sb.WriteString("\n\n")
	}

	if p.Author != "" {
		sb.WriteString("  " + dimStyle.Render("By: "))
		sb.WriteString(brightStyle.Render(p.Author))
		sb.WriteString("\n\n")
	}

	sb.WriteString("  " + warnStyle.Render("\u26a0 Make sure you trust a plugin before installing"))
	sb.WriteString("\n")

	frame.WriteString(shared.SelectorTitleStyle.Render("Install Plugin"))
	frame.WriteString("\n")
	frame.WriteString(shared.SelectorBreadcrumbStyle.Render("> " + p.Name + "@" + p.Marketplace))
	frame.WriteString("\n\n")
	frame.WriteString(s.renderViewport(sb.String(), s.detailScroll))
	s.renderActions(&frame)
	s.renderFooter(&frame, "\u2191/\u2193 scroll/actions \u00b7 enter select \u00b7 esc back")
	return s.renderBox(frame.String())
}

func (s *Model) renderMarketplaceDetail() string {
	if s.detailMarketplace == nil {
		return s.renderTabList()
	}

	var sb strings.Builder
	var frame strings.Builder
	m := s.detailMarketplace

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(brightStyle.Render(m.ID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(m.Source))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("  %d available plugins", m.Available))
	sb.WriteString("\n")

	if m.Installed > 0 {
		sb.WriteString("\n")
		sb.WriteString("  " + dimStyle.Render(fmt.Sprintf("Installed (%d):", m.Installed)))
		sb.WriteString("\n")
		for _, p := range coreplugin.DefaultRegistry.List() {
			if idx := strings.Index(p.Source, "@"); idx != -1 && p.Source[idx+1:] == m.ID {
				sb.WriteString("    " + shared.SelectorStatusConnected.Render("\u25cf") + " " + p.Name())
				sb.WriteString("\n")
			}
		}
	}

	frame.WriteString(shared.SelectorTitleStyle.Render("Marketplace Details"))
	frame.WriteString("\n")
	frame.WriteString(shared.SelectorBreadcrumbStyle.Render("> " + m.ID))
	frame.WriteString("\n\n")
	frame.WriteString(s.renderViewport(sb.String(), s.detailScroll))
	s.renderActions(&frame)
	s.renderFooter(&frame, "\u2191/\u2193 scroll/actions \u00b7 enter select \u00b7 esc back")
	return s.renderBox(frame.String())
}

func (s *Model) renderAddMarketplaceDialog() string {
	var sb strings.Builder
	cw := s.contentWidth()

	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Add Marketplace"))
	sb.WriteString("\n\n")

	sb.WriteString(dimStyle.Render("Enter marketplace source:"))
	sb.WriteString("\n\n")
	sb.WriteString(dimStyle.Render("Examples:"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 https://github.com/owner/repo"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 owner/repo (GitHub shorthand)"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  \u2022 ./path/to/marketplace (local)"))
	sb.WriteString("\n\n")

	maxInputLen := cw - 6
	inputLine := s.addMarketplaceInput + "\u2502"
	if len(inputLine) > maxInputLen {
		inputLine = "\u2026" + inputLine[len(inputLine)-maxInputLen+1:]
	}
	sb.WriteString(brightStyle.Render("> " + inputLine))
	sb.WriteString("\n")

	s.renderFooter(&sb, "enter add \u00b7 esc cancel")
	return s.renderBox(sb.String())
}

func (s *Model) renderBrowsePlugins() string {
	var sb strings.Builder
	dimStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	brightStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextBright)

	sb.WriteString(shared.SelectorTitleStyle.Render("Browse Marketplace"))
	sb.WriteString("\n")
	sb.WriteString(shared.SelectorBreadcrumbStyle.Render("> " + s.browseMarketplaceID))
	sb.WriteString("\n\n")

	sb.WriteString(brightStyle.Render(s.browseMarketplaceID))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("%d available plugins", len(s.browsePlugins))))
	sb.WriteString("\n\n")

	cw := s.contentWidth()
	if len(s.browsePlugins) == 0 {
		sb.WriteString(dimStyle.Render("  No plugins found"))
		sb.WriteString("\n")
	} else {
		visible := max(4, s.bodyHeight())
		endIdx := min(s.scrollOffset+visible, len(s.browsePlugins))

		if s.scrollOffset > 0 {
			sb.WriteString(shared.SelectorHintStyle.Render("  \u2191 more above"))
			sb.WriteString("\n")
		}

		for i := s.scrollOffset; i < endIdx; i++ {
			p := s.browsePlugins[i]

			icon := "\u25cb"
			iconStyle := dimStyle
			if p.Installed {
				icon = "\u25cf"
				iconStyle = shared.SelectorStatusConnected
			}

			sb.WriteString(pluginCursor(i == s.selectedIdx))
			sb.WriteString(iconStyle.Render(icon))
			sb.WriteString(" ")
			sb.WriteString(p.Name)
			sb.WriteString("\n")

			if p.Description != "" && i == s.selectedIdx {
				desc := shared.TruncateText(p.Description, cw-10)
				sb.WriteString(dimStyle.Render("    " + desc))
				sb.WriteString("\n")
			}
		}

		if endIdx < len(s.browsePlugins) {
			sb.WriteString(shared.SelectorHintStyle.Render("  \u2193 more below"))
			sb.WriteString("\n")
		}
	}

	s.renderFooter(&sb, "\u2191/\u2193 navigate \u00b7 enter details \u00b7 esc back")
	return s.renderBox(sb.String())
}

func (s *Model) renderActions(sb *strings.Builder) {
	sb.WriteString("\n")
	accentStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true).
		PaddingLeft(2)
	normalStyle := lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Text).
		PaddingLeft(2)

	for i, action := range s.actions {
		if i == s.actionIdx {
			sb.WriteString(accentStyle.Render("\u276f " + action.Label))
		} else {
			sb.WriteString(normalStyle.Render("  " + action.Label))
		}
		sb.WriteString("\n")
	}
}

func (s *Model) renderFooter(sb *strings.Builder, hint string) {
	sb.WriteString("\n")
	if s.isLoading {
		spinnerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Accent)
		sb.WriteString(spinnerStyle.Render("  \u25d0 " + s.loadingMsg))
		sb.WriteString("\n\n")
	} else if s.lastMessage != "" {
		if s.isError {
			sb.WriteString(shared.SelectorStatusError.Render("  \u26a0 " + s.lastMessage))
		} else {
			successStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)
			sb.WriteString(successStyle.Render("  \u2713 " + s.lastMessage))
		}
		sb.WriteString("\n\n")
	}
	sb.WriteString(s.renderHints(hint))
}

func (s *Model) renderHints(hint string) string {
	hintStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
	return hintStyle.Render(hint)
}

func (s *Model) renderViewport(content string, scroll int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	visible := s.bodyHeight()
	if visible <= 0 {
		return ""
	}
	if scroll < 0 {
		scroll = 0
	}
	maxScroll := max(0, len(lines)-visible)
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := min(len(lines), scroll+visible)
	view := lines
	if len(lines) > 0 {
		view = lines[scroll:end]
	}

	if len(view) < visible {
		for len(view) < visible {
			view = append(view, "")
		}
	}

	return strings.Join(view, "\n") + "\n"
}

func (s *Model) renderBox(content string) string {
	box := shared.SelectorBorderStyle.
		Width(s.boxWidth()).
		Height(s.boxHeight()).
		Render(content)
	return lipgloss.NewStyle().Padding(0, 1).Render(box)
}

func pluginStatusIconAndStyle(enabled bool) (string, lipgloss.Style) {
	if enabled {
		return "\u25cf", shared.SelectorStatusConnected
	}
	return "\u25cb", shared.SelectorStatusNone
}

func pluginCursor(selected bool) string {
	if selected {
		return "\u276f "
	}
	return "  "
}

func buildComponentList(p *PluginItem) []string {
	type componentCount struct {
		icon  string
		name  string
		count int
	}
	counts := []componentCount{
		{"\u2726", "Skills", p.Skills},
		{"\u2691", "Agents", p.Agents},
		{"\u2318", "Commands", p.Commands},
		{"\u21aa", "Hooks", p.Hooks},
		{"\u2609", "MCP Servers", p.MCP},
		{"\u29be", "LSP Servers", p.LSP},
	}

	var result []string
	for _, c := range counts {
		if c.count > 0 {
			result = append(result, fmt.Sprintf("%s %s: %d", c.icon, c.name, c.count))
		}
	}
	return result
}

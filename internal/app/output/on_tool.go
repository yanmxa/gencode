package output

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	coretool "github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// --- Tool state ---

// ToolState holds tool selector and execution state for the TUI model.
type ToolState struct {
	Selector ToolSelector
	ToolExecState
}

// ToolExecState holds tool execution state for the TUI model.
type ToolExecState struct {
	PendingCalls []core.ToolCall
	CurrentIdx   int
	Ctx          context.Context
	Cancel       context.CancelFunc
}

// Begin initializes a fresh execution context for a tool run and returns it.
func (t *ToolExecState) Begin() context.Context {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Ctx, t.Cancel = context.WithCancel(context.Background())
	return t.Ctx
}

// Context returns the active execution context, or Background when idle.
func (t *ToolExecState) Context() context.Context {
	if t.Ctx != nil {
		return t.Ctx
	}
	return context.Background()
}

// Reset clears all tool execution state.
func (t *ToolExecState) Reset() {
	if t.Cancel != nil {
		t.Cancel()
	}
	t.PendingCalls = nil
	t.CurrentIdx = 0
	t.Ctx = nil
	t.Cancel = nil
}

// --- Tool selector ---

type toolItem struct {
	Name        string
	Description string
	Enabled     bool
}

// ToolSelector holds state for the tool selector
type ToolSelector struct {
	active        bool
	tools         []toolItem
	filteredTools []toolItem
	nav           kit.ListNav
	width         int
	height        int
	disabledTools map[string]bool
	saveLevel     kit.SaveLevel
	loadDisabled  func(userLevel bool) map[string]bool
	saveDisabled  func(disabled map[string]bool, userLevel bool) error
}

// ToggleMsg is sent when a tool's enabled state is toggled
type ToggleMsg struct {
	ToolName string
	Enabled  bool
}

// NewToolSelector creates a new ToolSelector with injected load/save callbacks.
func NewToolSelector(
	loadDisabled func(userLevel bool) map[string]bool,
	saveDisabled func(disabled map[string]bool, userLevel bool) error,
) ToolSelector {
	return ToolSelector{
		active:       false,
		tools:        []toolItem{},
		nav:          kit.ListNav{MaxVisible: 10},
		loadDisabled: loadDisabled,
		saveDisabled: saveDisabled,
	}
}

// EnterSelect enters tool selection mode
func (s *ToolSelector) EnterSelect(width, height int, disabledTools map[string]bool, mcpTools func() []core.ToolSchema) error {
	allTools := coretool.GetToolSchemasWithMCP(mcpTools)

	s.tools = make([]toolItem, 0, len(allTools))
	for _, t := range allTools {
		s.tools = append(s.tools, toolItem{
			Name:        t.Name,
			Description: t.Description,
			Enabled:     !disabledTools[t.Name],
		})
	}

	s.active = true
	s.width = width
	s.height = height
	s.disabledTools = disabledTools
	s.filteredTools = s.tools
	s.nav.Reset()
	s.nav.Total = len(s.filteredTools)

	return nil
}

// IsActive returns whether the selector is active
func (s *ToolSelector) IsActive() bool {
	return s.active
}

// Cancel cancels the selector
func (s *ToolSelector) Cancel() {
	s.active = false
	s.tools = []toolItem{}
	s.filteredTools = []toolItem{}
	s.nav.Reset()
	s.nav.Total = 0
}

func (s *ToolSelector) updateFilter() {
	if s.nav.Search == "" {
		s.filteredTools = s.tools
	} else {
		query := strings.ToLower(s.nav.Search)
		s.filteredTools = make([]toolItem, 0)
		for _, t := range s.tools {
			if kit.FuzzyMatch(strings.ToLower(t.Name), query) ||
				kit.FuzzyMatch(strings.ToLower(t.Description), query) {
				s.filteredTools = append(s.filteredTools, t)
			}
		}
	}
	s.nav.ResetCursor()
	s.nav.Total = len(s.filteredTools)
}

func (s *ToolSelector) reloadToolStates() {
	levelDisabled := s.loadDisabled(s.saveLevel == kit.SaveLevelUser)

	for k := range s.disabledTools {
		delete(s.disabledTools, k)
	}
	for k, v := range levelDisabled {
		s.disabledTools[k] = v
	}

	for i := range s.tools {
		s.tools[i].Enabled = !s.disabledTools[s.tools[i].Name]
	}
	for i := range s.filteredTools {
		s.filteredTools[i].Enabled = !s.disabledTools[s.filteredTools[i].Name]
	}
}

// Toggle toggles the enabled state of the currently selected tool
func (s *ToolSelector) Toggle() tea.Cmd {
	if len(s.filteredTools) == 0 || s.nav.Selected >= len(s.filteredTools) {
		return nil
	}

	selected := &s.filteredTools[s.nav.Selected]
	selected.Enabled = !selected.Enabled

	for i := range s.tools {
		if s.tools[i].Name == selected.Name {
			s.tools[i].Enabled = selected.Enabled
			break
		}
	}

	if selected.Enabled {
		delete(s.disabledTools, selected.Name)
	} else {
		s.disabledTools[selected.Name] = true
	}

	_ = s.saveDisabled(s.disabledTools, s.saveLevel == kit.SaveLevelUser)

	return func() tea.Msg {
		return ToggleMsg{
			ToolName: selected.Name,
			Enabled:  selected.Enabled,
		}
	}
}

// HandleKeypress handles a keypress and returns a command if needed
func (s *ToolSelector) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyTab {
		if s.saveLevel == kit.SaveLevelProject {
			s.saveLevel = kit.SaveLevelUser
		} else {
			s.saveLevel = kit.SaveLevelProject
		}
		s.reloadToolStates()
		return nil
	}

	if key.Type == tea.KeyEnter {
		return s.Toggle()
	}

	searchChanged, consumed := s.nav.HandleKey(key)
	if searchChanged {
		s.updateFilter()
	}
	if consumed {
		return nil
	}

	if key.Type == tea.KeyEsc {
		s.Cancel()
		return func() tea.Msg { return kit.DismissedMsg{} }
	}

	return nil
}

// Render renders the tool selector
func (s *ToolSelector) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	levelIndicator := fmt.Sprintf("[%s]", s.saveLevel.String())
	title := fmt.Sprintf("Manage Tools (%d/%d)  %s", len(s.filteredTools), len(s.tools), levelIndicator)
	sb.WriteString(kit.SelectorTitleStyle().Render(title))
	sb.WriteString("\n")

	searchPrompt := "🔍 "
	if s.nav.Search == "" {
		sb.WriteString(kit.SelectorHintStyle().Render(searchPrompt + "Type to filter..."))
	} else {
		sb.WriteString(kit.SelectorBreadcrumbStyle().Render(searchPrompt + s.nav.Search + "▏"))
	}
	sb.WriteString("\n\n")

	boxWidth := kit.CalculateToolBoxWidth(s.width)
	maxDescLen := max(boxWidth-30, 20)

	if len(s.filteredTools) == 0 {
		sb.WriteString(kit.SelectorHintStyle().Render("  No tools match the filter"))
		sb.WriteString("\n")
	} else {
		startIdx, endIdx := s.nav.VisibleRange()

		if startIdx > 0 {
			sb.WriteString(kit.SelectorHintStyle().Render("  ↑ more above"))
			sb.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			t := s.filteredTools[i]

			var statusIcon string
			var statusStyle lipgloss.Style
			if t.Enabled {
				statusIcon = "●"
				statusStyle = kit.SelectorStatusConnected()
			} else {
				statusIcon = "○"
				statusStyle = kit.SelectorStatusNone()
			}

			desc := t.Description
			if idx := strings.Index(desc, "\n"); idx != -1 {
				desc = desc[:idx]
			}
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}

			descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
			line := fmt.Sprintf("%s %-15s  %s",
				statusStyle.Render(statusIcon),
				t.Name,
				descStyle.Render(desc),
			)

			if i == s.nav.Selected {
				sb.WriteString(kit.SelectorSelectedStyle().Render("> " + line))
			} else {
				sb.WriteString(kit.SelectorItemStyle().Render("  " + line))
			}
			sb.WriteString("\n")
		}

		if endIdx < len(s.filteredTools) {
			sb.WriteString(kit.SelectorHintStyle().Render("  ↓ more below"))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(kit.SelectorHintStyle().Render("↑/↓ navigate · Enter toggle · Tab level · Esc cancel"))

	content := sb.String()
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)

	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

// Lipgloss styles for tool result rendering, referencing theme directly.
var (
	headerStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(kit.CurrentTheme.Border).
			Padding(0, 1)

	headerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(kit.CurrentTheme.Primary)

	headerSubtitleStyle = lipgloss.NewStyle().
				Foreground(kit.CurrentTheme.Text)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Muted)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Muted).
			Width(5).
			Align(lipgloss.Right)

	matchStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Warning).
			Bold(true)

	filePathStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Primary)

	truncatedStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Muted).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(kit.CurrentTheme.Error)
)

// RenderToolResult renders a complete tool result with header and content.
func RenderToolResult(result toolresult.ToolResult, width int) string {
	if !result.Success {
		return renderErrorHeader(result.Metadata.Title, result.Error, width)
	}

	var sb strings.Builder

	sb.WriteString(renderHeader(result.Metadata, width))
	sb.WriteString("\n")

	switch result.Metadata.Title {
	case "Read":
		if len(result.Lines) > 0 {
			sb.WriteString(renderLines(result.Lines, true))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Glob":
		if len(result.Files) > 0 {
			sb.WriteString(renderFileList(result.Files, 20))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "Grep":
		if len(result.Lines) > 0 {
			sb.WriteString(renderGrepResults(result.Lines, 30))
		} else if result.Output != "" {
			sb.WriteString(result.Output)
		}
	case "WebFetch":
		if result.Output != "" {
			lines := strings.Split(result.Output, "\n")
			for _, line := range lines {
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	default:
		if result.Output != "" {
			sb.WriteString(result.Output)
		}
	}

	return sb.String()
}

// --- Header rendering ---

func renderHeader(meta toolresult.ResultMetadata, width int) string {
	title := headerTitleStyle.Render(meta.Title)
	subtitle := fmt.Sprintf("%s %s", meta.Icon, headerSubtitleStyle.Render(meta.Subtitle))

	metaParts := make([]string, 0, 6)
	if meta.Size > 0 {
		metaParts = append(metaParts, toolresult.FormatSize(meta.Size))
	}
	if meta.LineCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d lines", meta.LineCount))
	}
	if meta.ItemCount > 0 {
		switch meta.Title {
		case "Glob":
			metaParts = append(metaParts, fmt.Sprintf("%d files", meta.ItemCount))
		case "Grep":
			metaParts = append(metaParts, fmt.Sprintf("%d matches", meta.ItemCount))
		default:
			metaParts = append(metaParts, fmt.Sprintf("%d items", meta.ItemCount))
		}
	}
	if meta.StatusCode > 0 {
		metaParts = append(metaParts, fmt.Sprintf("%d OK", meta.StatusCode))
	}
	if meta.Duration > 0 {
		metaParts = append(metaParts, toolresult.FormatDuration(meta.Duration))
	}
	if meta.Truncated {
		metaParts = append(metaParts, truncatedStyle.Render("(truncated)"))
	}
	metaLine := headerMetaStyle.Render(strings.Join(metaParts, " · "))

	content := fmt.Sprintf("%s\n%s\n%s", title, subtitle, metaLine)
	box := headerStyle.Width(capBoxWidth(width) - 4).Render(content)
	return box
}

func renderErrorHeader(toolName, errorMsg string, width int) string {
	title := headerTitleStyle.Render(toolName)
	errorLine := fmt.Sprintf("%s %s", toolresult.IconError, errorStyle.Render("Error"))
	msgLine := errorStyle.Render(errorMsg)

	content := fmt.Sprintf("%s\n%s\n%s", title, errorLine, msgLine)

	errorBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(kit.CurrentTheme.Error).
		Padding(0, 1)

	box := errorBoxStyle.Width(capBoxWidth(width) - 4).Render(content)
	return box
}

func capBoxWidth(width int) int {
	if width <= 0 {
		return 50
	}
	maxWidth := width * 80 / 100
	if maxWidth < 50 {
		return 50
	}
	return maxWidth
}

// --- Content rendering ---

func renderLines(lines []toolresult.ContentLine, showLineNo bool) string {
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder

	maxLineNo := 0
	for _, line := range lines {
		if line.LineNo > maxLineNo {
			maxLineNo = line.LineNo
		}
	}
	lineNoWidth := len(fmt.Sprintf("%d", maxLineNo))
	if lineNoWidth < 4 {
		lineNoWidth = 4
	}

	for _, line := range lines {
		switch line.Type {
		case toolresult.LineTruncated:
			sb.WriteString(truncatedStyle.Render(line.Text))
			sb.WriteString("\n")
		default:
			if showLineNo && line.LineNo > 0 {
				lineNoStr := fmt.Sprintf("%*d", lineNoWidth, line.LineNo)
				sb.WriteString(lineNumberStyle.Render(lineNoStr))
				sb.WriteString(lineNumberStyle.Render("│"))
			} else if showLineNo {
				sb.WriteString(strings.Repeat(" ", lineNoWidth))
				sb.WriteString(lineNumberStyle.Render("│"))
			}

			var content string
			switch line.Type {
			case toolresult.LineMatch:
				content = matchStyle.Render(line.Text)
			case toolresult.LineHeader:
				content = filePathStyle.Render(line.Text)
			default:
				content = line.Text
			}
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func renderFileList(files []string, maxShow int) string {
	if len(files) == 0 {
		return truncatedStyle.Render("  (no files found)\n")
	}

	var sb strings.Builder
	showCount := len(files)
	truncated := false
	if maxShow > 0 && showCount > maxShow {
		showCount = maxShow
		truncated = true
	}

	for i := 0; i < showCount; i++ {
		sb.WriteString("  ")
		sb.WriteString(filePathStyle.Render(files[i]))
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(files) - maxShow
		sb.WriteString(truncatedStyle.Render(fmt.Sprintf("  ... and %d more files\n", remaining)))
	}

	return sb.String()
}

func renderGrepResults(lines []toolresult.ContentLine, maxShow int) string {
	if len(lines) == 0 {
		return truncatedStyle.Render("  (no matches found)\n")
	}

	var sb strings.Builder
	showCount := len(lines)
	truncated := false
	if maxShow > 0 && showCount > maxShow {
		showCount = maxShow
		truncated = true
	}

	for i := 0; i < showCount; i++ {
		line := lines[i]
		sb.WriteString("  ")
		if line.File != "" {
			sb.WriteString(filePathStyle.Render(line.File))
			sb.WriteString(":")
		}
		if line.LineNo > 0 {
			sb.WriteString(lineNumberStyle.Render(fmt.Sprintf("%d", line.LineNo)))
			sb.WriteString(": ")
		}
		sb.WriteString(line.Text)
		sb.WriteString("\n")
	}

	if truncated {
		remaining := len(lines) - maxShow
		sb.WriteString(truncatedStyle.Render(fmt.Sprintf("  ... and %d more matches\n", remaining)))
	}

	return sb.String()
}

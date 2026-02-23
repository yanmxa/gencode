package suggest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tui/theme"
)

type Type int

const (
	TypeCommand Type = iota
	TypeFile
)

type Suggestion struct {
	Name        string
	Description string
}

type Matcher func(query string) []Suggestion

var (
	suggestionBoxStyle      lipgloss.Style
	selectedSuggestionStyle lipgloss.Style
	normalSuggestionStyle   lipgloss.Style
	commandNameStyle        lipgloss.Style
	commandDescStyle        lipgloss.Style
)

func init() {
	suggestionBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.CurrentTheme.Border).
		Padding(0, 1)

	selectedSuggestionStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Bold(true)

	normalSuggestionStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)

	commandNameStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary)

	commandDescStyle = lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Muted)
}

type FileSuggestion struct {
	Path        string
	DisplayName string
	IsDir       bool
}

type State struct {
	visible         bool
	suggestionType  Type
	suggestions     []Suggestion
	fileSuggestions []FileSuggestion
	selectedIdx     int
	cwd             string
	atQuery         string
	cmdMatcher      Matcher
}

func NewState(matcher Matcher) State {
	return State{
		visible:    false,
		cmdMatcher: matcher,
	}
}

func (s *State) Reset() {
	s.visible = false
	s.suggestions = nil
	s.fileSuggestions = nil
	s.selectedIdx = 0
	s.atQuery = ""
}

func (s *State) UpdateSuggestions(input string) {
	input = strings.TrimSpace(input)

	if atIdx := strings.LastIndex(input, "@"); atIdx >= 0 {
		query := input[atIdx+1:]
		if atIdx == len(input)-1 || !strings.ContainsAny(query, " \t\n") {
			s.atQuery = query
			s.updateFileSuggestions(query)
			return
		}
	}

	if strings.HasPrefix(input, "/") {
		s.suggestionType = TypeCommand
		s.suggestions = s.cmdMatcher(input)
		s.fileSuggestions = nil
		s.visible = len(s.suggestions) > 0
		s.atQuery = ""

		if s.selectedIdx >= len(s.suggestions) {
			s.selectedIdx = 0
		}
		return
	}

	s.visible = false
	s.suggestions = nil
	s.fileSuggestions = nil
	s.selectedIdx = 0
	s.atQuery = ""
}

const (
	fileScanMaxResults = 10
	fileScanMaxDepth   = 4
	fileScanMaxDisplay = 8
)

func (s *State) updateFileSuggestions(query string) {
	s.suggestionType = TypeFile
	s.suggestions = nil
	s.fileSuggestions = nil

	if s.cwd == "" {
		s.visible = false
		return
	}

	s.fileSuggestions = s.scanMarkdownFiles(query)
	s.sortAndLimitSuggestions()

	s.visible = len(s.fileSuggestions) > 0
	if s.selectedIdx >= len(s.fileSuggestions) {
		s.selectedIdx = 0
	}
}

var supportedFileExtensions = map[string]bool{
	".md":   true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

func (s *State) scanMarkdownFiles(query string) []FileSuggestion {
	queryLower := strings.ToLower(query)
	seen := make(map[string]bool)
	var results []FileSuggestion

	var walkDir func(dir string, depth int)
	walkDir = func(dir string, depth int) {
		if depth > fileScanMaxDepth || len(results) >= fileScanMaxResults {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		var subdirs []string
		for _, entry := range entries {
			if len(results) >= fileScanMaxResults {
				return
			}

			name := entry.Name()
			fullPath := filepath.Join(dir, name)

			if entry.IsDir() {
				if !shouldSkipDirectory(name) {
					subdirs = append(subdirs, fullPath)
				}
				continue
			}

			ext := strings.ToLower(filepath.Ext(name))
			if !supportedFileExtensions[ext] {
				continue
			}

			relPath, err := filepath.Rel(s.cwd, fullPath)
			if err != nil || seen[relPath] {
				continue
			}
			seen[relPath] = true

			if query != "" && !fuzzyMatchFile(strings.ToLower(relPath), queryLower) {
				continue
			}

			results = append(results, FileSuggestion{
				Path:        relPath,
				DisplayName: relPath,
				IsDir:       false,
			})
		}

		for _, subdir := range subdirs {
			walkDir(subdir, depth+1)
		}
	}

	walkDir(s.cwd, 0)
	return results
}

func (s *State) sortAndLimitSuggestions() {
	sort.Slice(s.fileSuggestions, func(i, j int) bool {
		depthI := strings.Count(s.fileSuggestions[i].Path, "/")
		depthJ := strings.Count(s.fileSuggestions[j].Path, "/")
		if depthI != depthJ {
			return depthI < depthJ
		}
		return len(s.fileSuggestions[i].Path) < len(s.fileSuggestions[j].Path)
	})

	if len(s.fileSuggestions) > fileScanMaxDisplay {
		s.fileSuggestions = s.fileSuggestions[:fileScanMaxDisplay]
	}
}

func shouldSkipDirectory(name string) bool {
	if strings.HasPrefix(name, ".") && name != ".gen" && name != ".claude" {
		return true
	}

	switch name {
	case "node_modules", "vendor", ".git", "__pycache__", "dist", "build":
		return true
	}
	return false
}

func fuzzyMatchFile(str, pattern string) bool {
	if pattern == "" {
		return true
	}
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func (s *State) SetCwd(cwd string) {
	s.cwd = cwd
}

func (s *State) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
	}
}

func (s *State) MoveDown() {
	maxIdx := len(s.suggestions) - 1
	if s.suggestionType == TypeFile {
		maxIdx = len(s.fileSuggestions) - 1
	}
	if s.selectedIdx < maxIdx {
		s.selectedIdx++
	}
}

func (s *State) GetSelected() string {
	if !s.visible {
		return ""
	}

	if s.suggestionType == TypeFile {
		if len(s.fileSuggestions) == 0 || s.selectedIdx >= len(s.fileSuggestions) {
			return ""
		}
		return s.fileSuggestions[s.selectedIdx].Path
	}

	if len(s.suggestions) == 0 || s.selectedIdx >= len(s.suggestions) {
		return ""
	}
	return "/" + s.suggestions[s.selectedIdx].Name
}

func (s *State) GetSuggestionType() Type {
	return s.suggestionType
}

func (s *State) GetAtQuery() string {
	return s.atQuery
}

func (s *State) Hide() {
	s.visible = false
}

func (s *State) IsVisible() bool {
	if s.suggestionType == TypeFile {
		return s.visible && len(s.fileSuggestions) > 0
	}
	return s.visible && len(s.suggestions) > 0
}

func (s *State) Render(width int) string {
	if !s.IsVisible() {
		return ""
	}

	if s.suggestionType == TypeFile {
		return s.renderFileSuggestions(width)
	}
	return s.renderCommandSuggestions(width)
}

func (s *State) renderFileSuggestions(width int) string {
	const maxItems = 8
	items := s.fileSuggestions
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	boxWidth := clampInt(width*60/100, 40, 60)

	var lines []string
	headerStyle := lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
	lines = append(lines, headerStyle.Render("@ Import file:"))

	maxPathLen := boxWidth - 10
	for i, file := range items {
		icon := "📄"
		if file.IsDir {
			icon = "📁"
		}

		displayPath := truncateFromLeft(file.DisplayName, maxPathLen)
		line := fmt.Sprintf("%s %s", icon, displayPath)

		if i == s.selectedIdx {
			lines = append(lines, selectedSuggestionStyle.Render("> "+line))
		} else {
			lines = append(lines, normalSuggestionStyle.Render("  "+line))
		}
	}

	lines = append(lines, "", commandDescStyle.Render("Tab/Enter to select · Esc to cancel"))

	content := strings.Join(lines, "\n")
	return suggestionBoxStyle.Width(boxWidth).Render(content)
}

func (s *State) renderCommandSuggestions(width int) string {
	const maxItems = 5
	items := s.suggestions
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	maxWidth := max(width-4, 40)
	boxWidth := clampInt(width*80/100, 40, maxWidth)
	contentWidth := max(boxWidth-4, 20)

	var lines []string
	for i, cmd := range items {
		cmdName := "/" + cmd.Name
		maxDescLen := max(contentWidth-len(cmdName)-3, 10)
		desc := truncateWithEllipsis(cmd.Description, maxDescLen)

		line := fmt.Sprintf("%s - %s", cmdName, desc)

		if i == s.selectedIdx {
			lines = append(lines, selectedSuggestionStyle.Render(line))
		} else {
			lines = append(lines, commandNameStyle.Render(cmdName)+commandDescStyle.Render(" - "+desc))
		}
	}

	content := strings.Join(lines, "\n")
	return suggestionBoxStyle.Width(boxWidth).Render(content)
}

func clampInt(value, minVal, maxVal int) int {
	return max(minVal, min(value, maxVal))
}

func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func truncateFromLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[len(s)-maxLen:]
	}
	return "..." + s[len(s)-maxLen+3:]
}

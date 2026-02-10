package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SuggestionType indicates what kind of suggestion is being shown
type SuggestionType int

const (
	SuggestionTypeCommand SuggestionType = iota
	SuggestionTypeFile
)

// Suggestion styles (initialized dynamically based on theme)
var (
	suggestionBoxStyle      lipgloss.Style
	selectedSuggestionStyle lipgloss.Style
	normalSuggestionStyle   lipgloss.Style
	commandNameStyle        lipgloss.Style
	commandDescStyle        lipgloss.Style
)

func init() {
	// Initialize suggestion styles based on current theme
	suggestionBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CurrentTheme.Border).
		Padding(0, 1)

	selectedSuggestionStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextBright).
		Bold(true)

	normalSuggestionStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	commandNameStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary)

	commandDescStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)
}

// FileSuggestion represents a file suggestion for @import
type FileSuggestion struct {
	Path        string // Relative path from cwd
	DisplayName string // Display name (shortened)
	IsDir       bool   // Is it a directory
}

// SuggestionState holds the state for command and file suggestions
type SuggestionState struct {
	visible         bool
	suggestionType  SuggestionType
	suggestions     []Command        // For command suggestions
	fileSuggestions []FileSuggestion // For file suggestions
	selectedIdx     int
	cwd             string // Current working directory for file scanning
	atQuery         string // The query after @ for file matching
}

// NewSuggestionState creates a new SuggestionState
func NewSuggestionState() SuggestionState {
	return SuggestionState{
		visible:     false,
		suggestions: []Command{},
		selectedIdx: 0,
	}
}

// Reset resets the suggestion state
func (s *SuggestionState) Reset() {
	s.visible = false
	s.suggestions = nil
	s.fileSuggestions = nil
	s.selectedIdx = 0
	s.atQuery = ""
}

// UpdateSuggestions updates suggestions based on input
func (s *SuggestionState) UpdateSuggestions(input string) {
	input = strings.TrimSpace(input)

	// Check for @ file reference (look for last @ in input)
	if atIdx := strings.LastIndex(input, "@"); atIdx >= 0 {
		// Get the query after @
		query := input[atIdx+1:]
		// Only trigger if we're typing after @ (not a standalone @)
		if atIdx == len(input)-1 || !strings.ContainsAny(query, " \t\n") {
			s.atQuery = query
			s.updateFileSuggestions(query)
			return
		}
	}

	// Check for / command
	if strings.HasPrefix(input, "/") {
		s.suggestionType = SuggestionTypeCommand
		s.suggestions = GetMatchingCommands(input)
		s.fileSuggestions = nil
		s.visible = len(s.suggestions) > 0
		s.atQuery = ""

		if s.selectedIdx >= len(s.suggestions) {
			s.selectedIdx = 0
		}
		return
	}

	// No suggestions
	s.visible = false
	s.suggestions = []Command{}
	s.fileSuggestions = nil
	s.selectedIdx = 0
	s.atQuery = ""
}

// File suggestion configuration
const (
	fileScanMaxResults = 10
	fileScanMaxDepth   = 4
	fileScanMaxDisplay = 8
)

// updateFileSuggestions updates file suggestions based on query
func (s *SuggestionState) updateFileSuggestions(query string) {
	s.suggestionType = SuggestionTypeFile
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

// supportedFileExtensions defines file extensions for @ suggestions
var supportedFileExtensions = map[string]bool{
	".md":   true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

// scanMarkdownFiles scans for markdown and image files matching the query.
func (s *SuggestionState) scanMarkdownFiles(query string) []FileSuggestion {
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

			// Check if file has a supported extension
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

// sortAndLimitSuggestions sorts by depth then length, and limits results.
func (s *SuggestionState) sortAndLimitSuggestions() {
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

// shouldSkipDirectory returns true if the directory should be skipped during file scanning
func shouldSkipDirectory(name string) bool {
	// Skip hidden directories except .gen and .claude
	if strings.HasPrefix(name, ".") && name != ".gen" && name != ".claude" {
		return true
	}

	switch name {
	case "node_modules", "vendor", ".git", "__pycache__", "dist", "build":
		return true
	}
	return false
}

// fuzzyMatchFile checks if pattern chars appear in str in order
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

// SetCwd sets the current working directory for file scanning
func (s *SuggestionState) SetCwd(cwd string) {
	s.cwd = cwd
}

// MoveUp moves the selection up
func (s *SuggestionState) MoveUp() {
	if s.selectedIdx > 0 {
		s.selectedIdx--
	}
}

// MoveDown moves the selection down
func (s *SuggestionState) MoveDown() {
	maxIdx := len(s.suggestions) - 1
	if s.suggestionType == SuggestionTypeFile {
		maxIdx = len(s.fileSuggestions) - 1
	}
	if s.selectedIdx < maxIdx {
		s.selectedIdx++
	}
}

// GetSelected returns the currently selected suggestion, or empty string if none
func (s *SuggestionState) GetSelected() string {
	if !s.visible {
		return ""
	}

	if s.suggestionType == SuggestionTypeFile {
		if len(s.fileSuggestions) == 0 || s.selectedIdx >= len(s.fileSuggestions) {
			return ""
		}
		return s.fileSuggestions[s.selectedIdx].Path
	}

	// Command suggestion
	if len(s.suggestions) == 0 || s.selectedIdx >= len(s.suggestions) {
		return ""
	}
	return "/" + s.suggestions[s.selectedIdx].Name
}

// GetSuggestionType returns the current suggestion type
func (s *SuggestionState) GetSuggestionType() SuggestionType {
	return s.suggestionType
}

// GetAtQuery returns the current @ query
func (s *SuggestionState) GetAtQuery() string {
	return s.atQuery
}

// Hide hides the suggestions
func (s *SuggestionState) Hide() {
	s.visible = false
}

// IsVisible returns whether suggestions are visible
func (s *SuggestionState) IsVisible() bool {
	if s.suggestionType == SuggestionTypeFile {
		return s.visible && len(s.fileSuggestions) > 0
	}
	return s.visible && len(s.suggestions) > 0
}

// Render renders the suggestions box
func (s *SuggestionState) Render(width int) string {
	if !s.IsVisible() {
		return ""
	}

	// Render based on suggestion type
	if s.suggestionType == SuggestionTypeFile {
		return s.renderFileSuggestions(width)
	}
	return s.renderCommandSuggestions(width)
}

// renderFileSuggestions renders file suggestions for @import
func (s *SuggestionState) renderFileSuggestions(width int) string {
	const maxItems = 8
	items := s.fileSuggestions
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	boxWidth := clampInt(width*60/100, 40, 60)

	var lines []string
	headerStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
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

// renderCommandSuggestions renders command suggestions
func (s *SuggestionState) renderCommandSuggestions(width int) string {
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

// clampInt clamps a value between minVal and maxVal.
func clampInt(value, minVal, maxVal int) int {
	return max(minVal, min(value, maxVal))
}

// truncateWithEllipsis truncates a string and adds ellipsis if needed.
func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// truncateFromLeft truncates a string from the left, keeping the end visible.
func truncateFromLeft(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[len(s)-maxLen:]
	}
	return "..." + s[len(s)-maxLen+3:]
}

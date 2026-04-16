// Package searchui provides the search engine provider kit.
package searchui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/provider/search"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// item represents a search provider in the kit.
type item struct {
	Name        search.ProviderName
	DisplayName string
	EnvVars     []string
	Available   bool
	IsCurrent   bool
}

// SelectedMsg is sent when a search provider is selected.
type SelectedMsg struct {
	Provider search.ProviderName
}

// Model holds state for the search engine kit.
type Model struct {
	active      bool
	items       []item
	selectedIdx int
	width       int
	height      int
	store       *provider.Store
}

// New creates a new search selector Model.
func New() Model {
	return Model{}
}

// Enter activates the search kit.
func (s *Model) Enter(store *provider.Store, width, height int) error {
	if store == nil {
		var err error
		store, err = provider.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open provider store: %w", err)
		}
	}

	currentName := store.GetSearchProvider()
	if currentName == "" {
		currentName = string(search.ProviderExa) // default
	}

	allMeta := search.AllProviders()
	s.items = make([]item, 0, len(allMeta))
	for _, meta := range allMeta {
		available := !meta.RequiresAPIKey
		if !available {
			for _, envVar := range meta.EnvVars {
				if os.Getenv(envVar) != "" {
					available = true
					break
				}
			}
		}
		s.items = append(s.items, item{
			Name:        meta.Name,
			DisplayName: meta.DisplayName,
			EnvVars:     meta.EnvVars,
			Available:   available,
			IsCurrent:   string(meta.Name) == currentName,
		})
	}

	s.active = true
	s.selectedIdx = 0
	s.width = width
	s.height = height
	s.store = store

	// Pre-select current provider
	for i, item := range s.items {
		if item.IsCurrent {
			s.selectedIdx = i
			break
		}
	}

	return nil
}

// IsActive returns whether the selector is active.
func (s *Model) IsActive() bool {
	return s.active
}

// Cancel closes the kit.
func (s *Model) Cancel() {
	s.active = false
	s.items = nil
	s.selectedIdx = 0
	s.store = nil
}

// Select selects the current provider and persists the choice.
func (s *Model) Select() tea.Cmd {
	if s.selectedIdx >= len(s.items) {
		return nil
	}

	selected := s.items[s.selectedIdx]
	if !selected.Available {
		return nil // can't select unavailable provider
	}

	if s.store != nil {
		_ = s.store.SetSearchProvider(string(selected.Name))
	}

	// Update current markers
	for i := range s.items {
		s.items[i].IsCurrent = s.items[i].Name == selected.Name
	}

	return func() tea.Msg {
		return SelectedMsg{Provider: selected.Name}
	}
}

// HandleKeypress handles keyboard input.
func (s *Model) HandleKeypress(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
		return nil
	case tea.KeyDown, tea.KeyCtrlN:
		if s.selectedIdx < len(s.items)-1 {
			s.selectedIdx++
		}
		return nil
	case tea.KeyEnter:
		return s.Select()
	case tea.KeyEsc:
		s.Cancel()
		return func() tea.Msg {
			return kit.DismissedMsg{}
		}
	}

	// Vim keys
	switch key.String() {
	case "j":
		if s.selectedIdx < len(s.items)-1 {
			s.selectedIdx++
		}
	case "k":
		if s.selectedIdx > 0 {
			s.selectedIdx--
		}
	}

	return nil
}

// Render renders the search engine kit.
func (s *Model) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder

	dimStyle := kit.SelectorDimStyle()

	// Title
	sb.WriteString(kit.SelectorTitleStyle().Render("Search Engine"))
	sb.WriteString("\n\n")

	// Provider list
	const nameCol = 20
	for i, item := range s.items {
		isSelected := i == s.selectedIdx

		marker := "[ ]"
		markerStyle := kit.SelectorStatusNone()
		if item.IsCurrent {
			marker = "[*]"
			markerStyle = kit.SelectorStatusConnected()
		}

		envInfo := ""
		if len(item.EnvVars) > 0 {
			envInfo = kit.RenderEnvVarStatus(item.EnvVars[0])
		} else {
			envInfo = dimStyle.Render("no key required")
		}

		line := kit.FormatAlignedRow(markerStyle.Render(marker), item.DisplayName, nameCol, envInfo)
		sb.WriteString(kit.RenderSelectableRow(line, isSelected))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ navigate · Enter select · Esc cancel"))

	content := sb.String()
	boxWidth := kit.CalculateToolBoxWidth(s.width)
	box := kit.SelectorBorderStyle().Width(boxWidth).Render(content)

	return lipgloss.Place(s.width, s.height-4, lipgloss.Center, lipgloss.Center, box)
}

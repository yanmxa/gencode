package input

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/search"
	"github.com/yanmxa/gencode/internal/setting"
)

type searchItem struct {
	Name        search.ProviderName
	DisplayName string
	EnvVars     []string
	Available   bool
	IsCurrent   bool
}

type SearchSelectedMsg struct {
	Provider search.ProviderName
}

type SearchSelector struct {
	active      bool
	items       []searchItem
	selectedIdx int
	width       int
	height      int
	store       *llm.Store
	settingSvc  setting.Service
}

func NewSearchSelector(settingSvc setting.Service) SearchSelector {
	return SearchSelector{settingSvc: settingSvc}
}

func (s *SearchSelector) Enter(store *llm.Store, width, height int) error {
	if store == nil {
		var err error
		store, err = llm.NewStore()
		if err != nil {
			return fmt.Errorf("failed to open provider store: %w", err)
		}
	}

	currentName := store.GetSearchProvider()
	if currentName == "" {
		currentName = string(search.ProviderExa)
	}

	allMeta := search.AllProviders()
	s.items = make([]searchItem, 0, len(allMeta))
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
		s.items = append(s.items, searchItem{
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

	for i, item := range s.items {
		if item.IsCurrent {
			s.selectedIdx = i
			break
		}
	}

	return nil
}

func (s *SearchSelector) IsActive() bool {
	return s.active
}

func (s *SearchSelector) Cancel() {
	s.active = false
	s.items = nil
	s.selectedIdx = 0
	s.store = nil
}

func (s *SearchSelector) Select() tea.Cmd {
	if s.selectedIdx >= len(s.items) {
		return nil
	}

	selected := s.items[s.selectedIdx]
	if !selected.Available {
		return nil
	}

	if s.settingSvc != nil {
		s.settingSvc.SetSearchProvider(string(selected.Name))
	}
	if s.store != nil {
		_ = s.store.SetSearchProvider(string(selected.Name))
	}

	for i := range s.items {
		s.items[i].IsCurrent = s.items[i].Name == selected.Name
	}

	return func() tea.Msg {
		return SearchSelectedMsg{Provider: selected.Name}
	}
}

func (s *SearchSelector) HandleKeypress(key tea.KeyMsg) tea.Cmd {
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

func (s *SearchSelector) Render() string {
	if !s.active {
		return ""
	}

	var sb strings.Builder
	dimStyle := kit.DimStyle()

	sb.WriteString(s.sepLine())
	sb.WriteString("\n")

	sb.WriteString(kit.SelectorTitleStyle().Render("Search Engine"))
	sb.WriteString("\n\n")

	var body strings.Builder
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
		body.WriteString(kit.RenderSelectableRow(line, isSelected))
		body.WriteString("\n")
	}
	sb.WriteString(s.renderViewport(body.String()))

	sb.WriteString("\n")
	sb.WriteString(s.sepLine())
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ navigate · Enter select · Esc cancel"))

	content := sb.String()
	cw := s.contentWidth()
	box := lipgloss.NewStyle().
		Width(cw).
		Height(s.boxHeight()).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(s.width, s.height-2, lipgloss.Center, lipgloss.Top, box)
}

func (s *SearchSelector) contentWidth() int {
	return max(60, s.width-6)
}

func (s *SearchSelector) boxHeight() int {
	return max(18, s.height-4)
}

func (s *SearchSelector) bodyHeight() int {
	return max(6, s.boxHeight()-10)
}

func (s *SearchSelector) renderViewport(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	visible := s.bodyHeight()
	if visible <= 0 {
		return ""
	}

	view := lines
	for len(view) < visible {
		view = append(view, "")
	}

	return strings.Join(view, "\n") + "\n"
}

func (s *SearchSelector) sepLine() string {
	sepStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	return sepStyle.Render(strings.Repeat("─", s.contentWidth()-8))
}

// --- Search Runtime ---

func UpdateSearch(deps OverlayDeps, state *SearchSelector, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case SearchSelectedMsg:
		state.Cancel()
		deps.State.Provider.SetStatusMessage(fmt.Sprintf("Search engine: %s", msg.Provider))
		return kit.StatusTimer(3 * time.Second), true
	}
	return nil, false
}

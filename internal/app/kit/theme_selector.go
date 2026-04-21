package kit

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

)

type themeSelectedMsg struct {
	Theme string
}

func themeTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).MarginBottom(1)
}

func themeActiveStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(CurrentTheme.Primary)
}

func themeItemStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Text)
}

func themeDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
}

func themeHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.TextDim).MarginTop(1)
}

var themeChoices = []struct {
	label, value, desc string
}{
	{"Light", "light", "Light background terminal"},
	{"Dark", "dark", "Dark background terminal"},
}

type themeSelectorModel struct{ cursor int }

func newThemeSelector() themeSelectorModel { return themeSelectorModel{} }

func (m themeSelectorModel) Init() tea.Cmd { return nil }

func (m themeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(themeChoices)-1 {
			m.cursor++
		}
	case "enter", " ":
		return m, func() tea.Msg { return themeSelectedMsg{Theme: themeChoices[m.cursor].value} }
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m themeSelectorModel) View() string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s\n\n", themeTitleStyle().Render("Choose a color theme"))

	for i, opt := range themeChoices {
		cursor := "  "
		style := themeItemStyle()
		if i == m.cursor {
			cursor = "▶ "
			style = themeActiveStyle()
		}
		fmt.Fprintf(&s, "%s%s  %s\n", cursor, style.Render(opt.label), themeDescStyle().Render(opt.desc))
	}

	s.WriteString(themeHintStyle().Render("\n↑/↓ to move · enter to confirm · q to quit"))
	return s.String()
}

type themeCapture struct {
	inner    themeSelectorModel
	selected string
}

func (c themeCapture) Init() tea.Cmd { return c.inner.Init() }

func (c themeCapture) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sel, ok := msg.(themeSelectedMsg); ok {
		c.selected = sel.Theme
		return c, tea.Quit
	}
	inner, cmd := c.inner.Update(msg)
	c.inner = inner.(themeSelectorModel)
	return c, cmd
}

func (c themeCapture) View() string { return c.inner.View() }

// RunThemeSelector opens the theme selector and returns the chosen theme ("light" or "dark").
// Returns an empty string if the user quit without selecting.
func RunThemeSelector() (string, error) {
	p := tea.NewProgram(themeCapture{inner: newThemeSelector()})
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if fc, ok := final.(themeCapture); ok {
		return fc.selected, nil
	}
	return "", nil
}

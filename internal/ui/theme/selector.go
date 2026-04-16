// Theme selector provides a one-time TUI for choosing the color theme.
package theme

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// themeSelectedMsg is emitted when the user confirms a theme choice.
type themeSelectedMsg struct {
	Theme string // "light" or "dark"
}

var (
	selectorTitleStyle  = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	selectorActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(CurrentTheme.Primary)
	selectorItemStyle   = lipgloss.NewStyle().Foreground(CurrentTheme.Text)
	selectorDescStyle   = lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
	selectorHintStyle   = lipgloss.NewStyle().Foreground(CurrentTheme.TextDim).MarginTop(1)
)

var choices = []struct {
	label, value, desc string
}{
	{"Light", "light", "Light background terminal"},
	{"Dark", "dark", "Dark background terminal"},
}

type selectorModel struct{ cursor int }

func newSelector() selectorModel { return selectorModel{} }

func (m selectorModel) Init() tea.Cmd { return nil }

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.cursor < len(choices)-1 {
			m.cursor++
		}
	case "enter", " ":
		return m, func() tea.Msg { return themeSelectedMsg{Theme: choices[m.cursor].value} }
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m selectorModel) View() string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s\n\n", selectorTitleStyle.Render("Choose a color theme"))

	for i, opt := range choices {
		cursor := "  "
		style := selectorItemStyle
		if i == m.cursor {
			cursor = "▶ "
			style = selectorActiveStyle
		}
		fmt.Fprintf(&s, "%s%s  %s\n", cursor, style.Render(opt.label), selectorDescStyle.Render(opt.desc))
	}

	s.WriteString(selectorHintStyle.Render("\n↑/↓ to move · enter to confirm · q to quit"))
	return s.String()
}

// capture wraps model to intercept themeSelectedMsg and quit the program.
type capture struct {
	inner    selectorModel
	selected string
}

func (c capture) Init() tea.Cmd { return c.inner.Init() }

func (c capture) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sel, ok := msg.(themeSelectedMsg); ok {
		c.selected = sel.Theme
		return c, tea.Quit
	}
	inner, cmd := c.inner.Update(msg)
	c.inner = inner.(selectorModel)
	return c, cmd
}

func (c capture) View() string { return c.inner.View() }

// Run opens the theme selector and returns the chosen theme ("light" or "dark").
// Returns an empty string if the user quit without selecting.
func RunSelector() (string, error) {
	p := tea.NewProgram(capture{inner: newSelector()})
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if fc, ok := final.(capture); ok {
		return fc.selected, nil
	}
	return "", nil
}
